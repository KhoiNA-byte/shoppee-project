package main

import (
	"context"
	"crypto/sha1"
	"fmt"
	"html/template"
	"strconv"

	// "net/url"
	// "image/png"
	// "encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	// "github.com/joho/godotenv"
	uuid "github.com/satori/go.uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	// "gopkg.in/mgo.v2"
)

type item struct {
	Screenshot string `bson:"screenshot,omitempty"`
	Name       string `bson:"name,omitempty"`
	Price      int    `bson:"price,omitempty"`
	Category   string `bson:"category,omitempty"`
	Seller     string `bson:"seller,omitempty"`
}

type user struct {
	Username string `bson:"username,omitempty"`
	Password string `bson:"password,omitempty"`
}

type userBalance struct {
	Username string `bson:"username,omitempty"`
	Balance  int    `bson:"balance,omitempty"`
}

type cartItem struct {
	Screenshot string `bson:"screenshot,omitempty"`
	Name       string `bson:"name,omitempty"`
	Price      int    `bson:"price,omitempty"`
	Category   string `bson:"category,omitempty"`
	Seller     string `bson:"seller,omitempty"`
	Quantity   int    `bson:"quantity,omitempty"`
}

type userCart struct {
	Username  string     `bson:"username,omitempty"`
	CartItems []cartItem `bson:"items,omitempty"`
}

var tpl *template.Template
var dbUsers = map[string]user{}
var dbSessions = map[string]string{}

func init() {
	tpl = template.Must(template.ParseGlob("templates/*.html"))
}

func main() {
	ctx := context.Background()
	client := ConnectDB(ctx)
	defer client.Disconnect(ctx)
	http.HandleFunc("/", index)
	http.HandleFunc("/login", login)
	http.HandleFunc("/signup", signup)
	http.HandleFunc("/logout", logout)
	http.HandleFunc("/createListing", createListing)
	http.HandleFunc("/viewListing", viewListing)
	http.HandleFunc("/detailListing", detailListing)
	http.HandleFunc("/addBalance", addBalance)
	http.HandleFunc("/addToCart", addToCart)

	// http.Handle("/templates/assets/", http.StripPrefix("/templates/assets", http.FileServer(http.Dir("./assets"))))
	http.Handle("/public/", http.StripPrefix("/public", http.FileServer(http.Dir("./public"))))
	http.Handle("/templates/", http.StripPrefix("/templates", http.FileServer(http.Dir("./templates"))))
	http.Handle("/favicon.ico", http.NotFoundHandler())
	http.ListenAndServe(":8080", nil)

}

func signup(res http.ResponseWriter, req *http.Request) {
	if AlreadyLoggedin(req) {
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}

	var u user
	var ub userBalance
	var errorMessage string
	if req.Method == http.MethodPost {

		//get form values
		un := req.FormValue("username")
		pw := req.FormValue("password")
		u = user{un, pw}
		ub = userBalance{un, 0}

		// save db
		// ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
		ctx := req.Context()
		database := ConnectDB(ctx).Database("shoppeeDB")
		shopcollection := database.Collection("Users")
		shopcollection1 := database.Collection("Balances")

		//username taken?
		filter := bson.D{{"username", u.Username}}
		result := shopcollection.FindOne(ctx, filter)

		var checkUser user
		err1 := result.Decode(&checkUser)
		if err1 != nil {
			insertBalance, err := shopcollection1.InsertOne(ctx, ub)
			if err != nil {
				log.Fatal(err)
			}
			insertResult, err := shopcollection.InsertOne(ctx, u)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Users:", insertResult.InsertedID, " ,Balance:", insertBalance.InsertedID)
		} else {
			errorMessage = "The username is already taken."
			// http.Error(res, "username already taken", http.StatusForbidden)
			templateinput := struct {
				User         user
				ErrorMessage string
			}{
				User:         u,
				ErrorMessage: errorMessage,
			}
			tpl.ExecuteTemplate(res, "signup.html", templateinput)
			return
		}

		//create session
		sID := uuid.NewV4()
		c := &http.Cookie{
			Name:  "session",
			Value: sID.String(),
		}
		http.SetCookie(res, c)
		dbSessions[c.Value] = u.Username

		dbUsers[un] = u

		//redirect
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}
	templateinput := struct {
		User         user
		ErrorMessage string
	}{
		User:         u,
		ErrorMessage: errorMessage,
	}
	tpl.ExecuteTemplate(res, "signup.html", templateinput)
}

func login(res http.ResponseWriter, req *http.Request) {
	if AlreadyLoggedin(req) {
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}
	var u user
	var errorMessage string

	if req.Method == http.MethodPost {

		//get form values
		un := req.FormValue("username")
		pw := req.FormValue("password")
		u = user{un, pw}

		//connect to DB
		ctx := req.Context()
		database := ConnectDB(ctx).Database("shoppeeDB")
		shopcollection := database.Collection("Users")

		//not matched
		filter := bson.D{
			{"username", un},
			{"password", pw},
		}
		var dbUser user
		err := shopcollection.FindOne(ctx, filter).Decode(&dbUser)

		if err != nil {
			errorMessage = "Username and/or password do not match."
			// http.Error(res, "username already taken", http.StatusForbidden)
			templateinput := struct {
				User         user
				ErrorMessage string
			}{
				User:         u,
				ErrorMessage: errorMessage,
			}
			tpl.ExecuteTemplate(res, "login.html", templateinput)
			return
		}

		//create session
		sID := uuid.NewV4()
		c := &http.Cookie{
			Name:  "session",
			Value: sID.String(),
		}
		http.SetCookie(res, c)
		dbSessions[c.Value] = u.Username

		dbUsers[un] = u

		//redirect
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}
	templateinput := struct {
		User         user
		ErrorMessage string
	}{
		User:         u,
		ErrorMessage: errorMessage,
	}
	tpl.ExecuteTemplate(res, "login.html", templateinput)
}

func logout(res http.ResponseWriter, req *http.Request) {
	if !AlreadyLoggedin(req) {
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}
	c, _ := req.Cookie("session")
	//delete session
	delete(dbSessions, c.Value)
	//remove cookie
	c = &http.Cookie{
		Name:   "session",
		Value:  "",
		MaxAge: -1,
	}
	http.SetCookie(res, c)

	http.Redirect(res, req, "/", http.StatusSeeOther)
}

func index(res http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost {
		//get form values
		key := req.FormValue("searchKey")
		http.Redirect(res, req, "/viewListing?searchKey="+url.QueryEscape(key)+"&minprice=&maxprice=&categoryKey=", http.StatusSeeOther)
		return
	}
	uBalance := getUserWithBalance(res, req)
	u := getUser(res, req)
	records := DisplayAllRecords(res, req)
	templateinput := struct {
		User         user
		Records      []item
		UserBalances userBalance
	}{
		User:         u,
		Records:      records[:15],
		UserBalances: uBalance,
	}
	tpl.ExecuteTemplate(res, "index.html", templateinput)
}
func createListing(res http.ResponseWriter, req *http.Request) {
	if !AlreadyLoggedin(req) {
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}
	var info item
	if req.Method == http.MethodPost {

		//process image
		mf, fh, err := req.FormFile("screenshot")
		if err != nil {
			fmt.Println(err)
		}
		defer mf.Close()

		//create sha for file name
		ext := strings.Split(fh.Filename, ".")[1]
		h := sha1.New()
		io.Copy(h, mf)
		fname := fmt.Sprintf("%x", h.Sum(nil)) + "." + ext

		//create new file
		wd, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
		}
		path := filepath.Join(wd, "public", "pics", fname)
		nf, err := os.Create(path)
		if err != nil {
			fmt.Println(err)
		}
		defer nf.Close()

		//copy
		mf.Seek(0, 0)
		io.Copy(nf, mf)

		// save db
		ctx := req.Context()
		database := ConnectDB(ctx).Database("shoppeeDB")
		shopcollection := database.Collection("Items")

		//get form values
		u := getUser(res, req)
		screenshot := fname
		name := req.FormValue("name")
		priceStr := req.FormValue("price")
		category := req.FormValue("category")
		seller := u.Username
		// Convert price to integer
		price, err := strconv.Atoi(priceStr)
		if err != nil {
			// Handle the error (e.g., return a response or log the issue)
			log.Printf("Invalid price value: %v", err)
			http.Error(res, "Invalid price value", http.StatusBadRequest)
			return
		}
		info = item{screenshot, name, price, category, seller}

		insertinfo, err := shopcollection.InsertOne(ctx, info)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Items:", insertinfo.InsertedID)

		//redirect
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}
	tpl.ExecuteTemplate(res, "createListing.html", info)
}

func addBalance(res http.ResponseWriter, req *http.Request) {
	if !AlreadyLoggedin(req) {
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}
	uBalance := getUserWithBalance(res, req)
	var balanceInfo userBalance
	var existingBalance userBalance
	var errorMessage string
	var successMessage string

	if req.Method == http.MethodPost {
		// save db
		ctx := req.Context()
		database := ConnectDB(ctx).Database("shoppeeDB")
		shopcollection := database.Collection("Balances")

		//get form values
		u := getUser(res, req)
		username := u.Username
		balanceStr := req.FormValue("addBalance")
		balance, err := strconv.Atoi(balanceStr)
		if err != nil {
			// Handle the error (e.g., return a response or log the issue)
			log.Printf("Invalid price value: %v", err)
			http.Error(res, "Invalid price value", http.StatusBadRequest)
			return
		}

		// Check if the balance is less than 10
		if balance < 10 {
			errorMessage = "The deposit amount must be more than 10.000₫."
			// Render the form again with the error message
			tpl.ExecuteTemplate(res, "addBalance.html", struct {
				User           user
				BalanceInfo    userBalance
				ErrorMessage   string
				SuccessMessage string
				UserBalances   userBalance
			}{
				User:           u,
				BalanceInfo:    balanceInfo,
				ErrorMessage:   errorMessage,
				SuccessMessage: successMessage,
				UserBalances:   uBalance,
			})
			return
		}

		filter := bson.D{{"username", u.Username}}
		result := shopcollection.FindOne(ctx, filter).Decode(&existingBalance)

		if result == nil {
			newBalance := balance + existingBalance.Balance
			balanceInfo = userBalance{username, newBalance}

			update := bson.M{"$set": bson.M{"username": username, "balance": newBalance}}
			_, err = shopcollection.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
			if err != nil {
				log.Printf("Error updating balance: %v", err)
				http.Error(res, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			successMessage = fmt.Sprintf("Balance updated successfully! New balance: %d.000₫", newBalance)
			log.Printf("Balance updated successfully for user: %s, new balance: %d.000₫", username, newBalance)
		}
	}
	u := getUser(res, req)

	templateinput := struct {
		User           user
		BalanceInfo    userBalance
		ErrorMessage   string
		SuccessMessage string
		UserBalances   userBalance
	}{
		User:           u,
		BalanceInfo:    balanceInfo,
		ErrorMessage:   errorMessage,
		SuccessMessage: successMessage,
		UserBalances:   uBalance,
	}

	tpl.ExecuteTemplate(res, "addBalance.html", templateinput)
}

func addToCart(res http.ResponseWriter, req *http.Request) {
	if !AlreadyLoggedin(req) {
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}

	u := getUser(res, req)
	// allRecords := DisplayAllRecords(res, req)
	uBalance := getUserWithBalance(res, req)
	uCart := getUserWithCart(res, req)

	templateinput := struct {
		User user
		// Records      []item
		UserBalances userBalance
		CartItems    []cartItem
	}{
		User: u,
		// Records:      allRecords,
		UserBalances: uBalance,
		CartItems:    uCart.CartItems,
	}

	tpl.ExecuteTemplate(res, "addToCart.html", templateinput)
}

func viewListing(res http.ResponseWriter, req *http.Request) {
	u := getUser(res, req)
	allRecords := DisplayAllRecords(res, req)
	searchRecords := SearchRecords(res, req)
	uBalance := getUserWithBalance(res, req)

	if searchRecords == nil {
		templateinput := struct {
			User         user
			Records      []item
			UserBalances userBalance
		}{
			User:         u,
			Records:      allRecords,
			UserBalances: uBalance,
		}
		tpl.ExecuteTemplate(res, "viewListing.html", templateinput)
	} else {
		templateinput := struct {
			User         user
			Records      []item
			UserBalances userBalance
		}{
			User:         u,
			Records:      searchRecords,
			UserBalances: uBalance,
		}
		tpl.ExecuteTemplate(res, "viewListing.html", templateinput)

	}
}

func detailListing(res http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost {
		formType := req.FormValue("formType")
		switch formType {
		case "Search":
			//get form values
			key := req.FormValue("searchKey")
			http.Redirect(res, req, "/viewListing?searchKey="+url.QueryEscape(key)+"&minprice=&maxprice=&categoryKey=", http.StatusSeeOther)
			return
		case "addToCart":
			if AlreadyLoggedin(req) {
				u := getUser(res, req)
				record := Handler(res, req)
				uBalance := getUserWithBalance(res, req)

				ctx := req.Context()
				database := ConnectDB(ctx).Database("shoppeeDB")
				cartCollection := database.Collection("Carts")

				username := u.Username
				quantityStr := req.FormValue("quantity")

				quantity, err := strconv.Atoi(quantityStr)
				if err != nil || quantity <= 0 {
					log.Println("Invalid quantity value")
					http.Error(res, "Invalid quantity value", http.StatusBadRequest)
					return
				}

				// Step 1: Check if the user already has a cart with the same screenshot
				filter := bson.M{"username": username, "items.screenshot": record.Screenshot}
				update := bson.M{
					"$inc": bson.M{"items.$.quantity": quantity}, // Increment the quantity if it exists
				}

				result, err := cartCollection.UpdateOne(ctx, filter, update)
				if err != nil {
					log.Printf("Error updating cart: %v", err)
					http.Error(res, "Failed to update cart", http.StatusInternalServerError)
					return
				}

				// Step 2: If no matching screenshot, add a new item to the cart
				if result.MatchedCount == 0 {
					filter = bson.M{"username": username}
					update = bson.M{
						"$setOnInsert": bson.M{"username": username}, // Create cart if it does not exist
						"$push": bson.M{"items": bson.M{
							"name":       record.Name,
							"screenshot": record.Screenshot,
							"price":      record.Price,
							"quantity":   quantity,
							"category":   record.Category,
							"seller":     record.Seller,
						}},
					}

					_, err = cartCollection.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
					if err != nil {
						log.Printf("Error adding new item to cart: %v", err)
						http.Error(res, "Failed to add item to cart", http.StatusInternalServerError)
						return
					}
				}

				log.Println("Item added to cart successfully")
				successMessage := "Item added to cart successfully!"
				templateinput := struct {
					User           user
					Record         item
					UserBalances   userBalance
					SuccessMessage string
				}{
					User:           u,
					Record:         record,
					UserBalances:   uBalance,
					SuccessMessage: successMessage,
				}

				tpl.ExecuteTemplate(res, "detailListing.html", templateinput)
				return
			} else {
				http.Redirect(res, req, "/login", http.StatusSeeOther)
				return
			}

		default:
			http.Error(res, "Invalid form submission", http.StatusBadRequest)
		}

	}
	u := getUser(res, req)
	record := Handler(res, req)
	uBalance := getUserWithBalance(res, req)

	templateinput := struct {
		User         user
		Record       item
		UserBalances userBalance
	}{
		User:         u,
		Record:       record,
		UserBalances: uBalance,
	}
	tpl.ExecuteTemplate(res, "detailListing.html", templateinput)
}

func getUser(res http.ResponseWriter, req *http.Request) user {
	//get cookie
	c, err := req.Cookie("session")
	if err != nil {
		sID := uuid.NewV4()
		c = &http.Cookie{
			Name:  "session",
			Value: sID.String(),
		}
	}
	http.SetCookie(res, c)

	//if user exist already, get user
	var u user
	if un, ok := dbSessions[c.Value]; ok {
		u = dbUsers[un]
	}
	return u
}

func AlreadyLoggedin(req *http.Request) bool {
	c, err := req.Cookie("session")
	if err != nil {
		return false
	}
	un := dbSessions[c.Value]
	_, ok := dbUsers[un]
	return ok
}

func ConnectDB(ctx context.Context) *mongo.Client {
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb+srv://shoppeeDB:training@atlascluster.sqpyiwf.mongodb.net/?retryWrites=true&w=majority"))
	if err != nil {
		log.Fatal(err)
	}
	err = client.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	return client
}

func DisplayAllRecords(res http.ResponseWriter, req *http.Request) []item {

	// save db
	ctx := req.Context()
	database := ConnectDB(ctx).Database("shoppeeDB")
	shopcollection := database.Collection("Items")

	//find records
	//pass these options to the Find method
	findOptions := options.Find()
	//Set the limit of the number of record to find
	findOptions.SetLimit(20)
	//Define an array in which you can store the decoded documents
	var results []item

	//Passing the bson.D{{}} as the filter matches  documents in the collection
	cur, err := shopcollection.Find(ctx, bson.D{{}}, findOptions)
	if err != nil {
		log.Fatal(err)
	}
	//Finding multiple documents returns a cursor
	//Iterate through the cursor allows us to decode documents one at a time

	for cur.Next(ctx) {
		//Create a value into which the single document can be decoded
		var elem item
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}
		results = append(results, elem)
	}

	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}

	//Close the cursor once finished
	cur.Close(context.TODO())
	// fmt.Printf("Found multiple documents: %+v\n", results)
	// fmt.Println()
	return results
}

func SearchRecords(res http.ResponseWriter, req *http.Request) []item {
	//save db
	ctx := req.Context()
	database := ConnectDB(ctx).Database("shoppeeDB")
	shopcollection := database.Collection("Items")
	var allResults [][]item // A slice of slices to store results of each filter
	searchPerformed := 0    // Counter to check how many search criteria are used

	// <!-- ======================================== -->
	// <!-- ========== FROM SEARCHBAR ============== -->
	// <!-- ======================================== -->
	//get user input
	searchKey := req.FormValue("searchKey")
	fmt.Println(searchKey)
	if searchKey != "" {
		// create index on db (exist only 1, can change on db in index tab)
		model := mongo.IndexModel{Keys: bson.D{{"name", "text"}}}
		name, err := shopcollection.Indexes().CreateOne(ctx, model)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Name of index created: " + name)

		//filter and find
		filter := bson.D{{"$text", bson.D{{"$search", searchKey}}}}
		cursor, err := shopcollection.Find(ctx, filter)
		if err != nil {
			log.Fatal(err)
		}

		var results []item
		if err = cursor.All(ctx, &results); err != nil {
			log.Fatal(err)
		}

		// fmt.Printf("Found multiple documents: %+v\n", results)
		allResults = append(allResults, results)
		searchPerformed++
		fmt.Println(allResults)
	}
	// fmt.Println(results)
	// <!-- ======================================== -->
	// <!-- ========== FROM CATEGORY =============== -->
	// <!-- ======================================== -->
	categoryKey := req.FormValue("categoryKey")
	fmt.Println(categoryKey)
	if categoryKey != "" {
		//filter and find
		filter := bson.D{{"category", categoryKey}}
		// opts := options.Find().SetSort(bson.D{{"price", -1}})
		cursor, err := shopcollection.Find(ctx, filter)
		if err != nil {
			log.Fatal(err)
		}

		var results []item
		if err = cursor.All(ctx, &results); err != nil {
			log.Fatal(err)
		}

		// fmt.Printf("Found multiple documents: %+v\n", results)
		allResults = append(allResults, results)
		searchPerformed++
		fmt.Println(allResults)

	}

	// <!-- ======================================== -->
	// <!-- ========== FROM TAB ==================== -->
	// <!-- ======================================== -->
	//
	//
	//
	// <!-- ======================================== -->
	// <!-- ========== FROM PRICE ================== -->
	// <!-- ======================================== -->
	minPriceStr := req.FormValue("minprice")
	maxPriceStr := req.FormValue("maxprice")

	// Convert minPrice and maxPrice to integers
	minPrice, err := strconv.Atoi(minPriceStr)
	if err != nil && minPriceStr != "" {
		log.Fatal("Invalid min price:", err)
	}

	maxPrice := 9999999999999 // Default max price
	if maxPriceStr != "" {
		maxPrice, err = strconv.Atoi(maxPriceStr)
		if err != nil {
			log.Fatal("Invalid max price:", err)
		}
	}

	// Construct MongoDB filter
	filter := bson.M{}
	if minPrice > 0 {
		filter["price"] = bson.M{"$gte": minPrice}
	}
	if maxPrice < 9999999999999 {
		if filter["price"] != nil {
			filter["price"].(bson.M)["$lte"] = maxPrice
		} else {
			filter["price"] = bson.M{"$lte": maxPrice}
		}
	}

	// Query MongoDB
	cursor, err := shopcollection.Find(ctx, filter)
	if err != nil {
		log.Fatalf("Failed to execute query: %v", err)
	}

	var results []item
	if err = cursor.All(ctx, &results); err != nil {
		log.Fatalf("Failed to decode results: %v", err)
	}

	// Append results
	allResults = append(allResults, results)
	if minPrice != 0 || maxPrice != 9999999999999 {
		searchPerformed++
	}
	fmt.Println(allResults)

	// <!-- ======================================== -->
	// <!-- ========== SORT PRICE ================== -->
	// <!-- ======================================== -->

	// Find common items across all search results
	commonResults := findCommonItems(allResults, searchPerformed)
	return commonResults
}

func findCommonItems(allResults [][]item, searchPerformed int) []item {
	itemCount := make(map[string]int)
	uniqueItems := make(map[string]item)

	for _, results := range allResults {
		for _, itm := range results {
			// Assuming Name is a unique identifier; otherwise, use a different unique field
			if _, exists := uniqueItems[itm.Screenshot]; !exists {
				uniqueItems[itm.Screenshot] = itm
			}
			itemCount[itm.Screenshot]++
		}
	}

	// Collect items that appear in all search results
	var commonResults []item
	for name, count := range itemCount {
		if count == searchPerformed {
			commonResults = append(commonResults, uniqueItems[name])
		}
	}

	return commonResults
}

func getUserWithBalance(res http.ResponseWriter, req *http.Request) userBalance {

	// Connect to the database
	u := getUser(res, req)
	if u.Username == "" {
		return userBalance{}
	}
	ctx := req.Context()
	database := ConnectDB(ctx).Database("shoppeeDB")
	shopcollection := database.Collection("Balances")

	// Query the database for the user's balance
	filter := bson.D{{"username", u.Username}}
	var result userBalance
	err := shopcollection.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		log.Fatal(err)
	}

	// Return user with balance
	return result
}

func getUserWithCart(res http.ResponseWriter, req *http.Request) userCart {

	// Connect to the database
	u := getUser(res, req)
	if u.Username == "" {
		return userCart{}
	}
	ctx := req.Context()
	database := ConnectDB(ctx).Database("shoppeeDB")
	shopcollection := database.Collection("Carts")

	// Query the database for the user's balance
	filter := bson.D{{"username", u.Username}}
	var result userCart
	err := shopcollection.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		log.Fatal(err)
	}

	// Return user with balance
	return result
}

func Handler(res http.ResponseWriter, req *http.Request) item {
	//save db
	ctx := req.Context()
	database := ConnectDB(ctx).Database("shoppeeDB")
	shopcollection := database.Collection("Items")

	fmt.Println()
	screenshotId := req.FormValue("screenshotId")
	fmt.Println("the chosen item: " + screenshotId)

	//filter and find
	filter := bson.D{{"screenshot", screenshotId}}
	result := shopcollection.FindOne(ctx, filter)

	var detailItem item
	err := result.Decode(&detailItem)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(detailItem)
	return detailItem
}
