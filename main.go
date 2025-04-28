package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"strconv"
	"time"

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
	Username   string     `bson:"username,omitempty"`
	CartItems  []cartItem `bson:"items,omitempty"`
	TotalItems int
}
type historyDocument struct {
	Username    string     `bson:"username"`
	Items       []cartItem `bson:"items"`
	Total       int        `bson:"total"`
	PurchasedAt time.Time  `bson:"purchasedAt"`
}

var tpl *template.Template
var dbUsers = map[string]user{}
var dbSessions = map[string]string{}

func init() {
	tpl = template.Must(template.New("").Funcs(template.FuncMap{
		"mul": func(a, b int) int {
			return a * b
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
	}).ParseGlob("templates/*.html"))
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
	http.HandleFunc("/historyBuy", historyBuy)

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
	uCart := getUserWithCart(res, req)
	records, _ := DisplayAllRecords(res, req, 1, 15)
	templateinput := struct {
		User         user
		Records      []item
		UserBalances userBalance
		UserCart     userCart
	}{
		User:         u,
		Records:      records,
		UserBalances: uBalance,
		UserCart:     uCart,
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
	uCart := getUserWithCart(res, req)
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
				UserCart       userCart
			}{
				User:           u,
				BalanceInfo:    balanceInfo,
				ErrorMessage:   errorMessage,
				SuccessMessage: successMessage,
				UserBalances:   uBalance,
				UserCart:       uCart,
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
		UserCart       userCart
	}{
		User:           u,
		BalanceInfo:    balanceInfo,
		ErrorMessage:   errorMessage,
		SuccessMessage: successMessage,
		UserBalances:   uBalance,
		UserCart:       uCart,
	}

	tpl.ExecuteTemplate(res, "addBalance.html", templateinput)
}

func addToCart(res http.ResponseWriter, req *http.Request) {
	if !AlreadyLoggedin(req) {
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}

	u := getUser(res, req)
	uBalance := getUserWithBalance(res, req)
	uCart := getUserWithCart(res, req)

	successMessage := ""

	if req.Method == http.MethodPost {
		ctx := req.Context()
		database := ConnectDB(ctx).Database("shoppeeDB")
		shopcollection := database.Collection("Carts")
		shopcollection1 := database.Collection("Balances")

		if itemName := req.FormValue("update"); itemName != "" {
			quantityStr := req.FormValue("quantity-" + itemName)
			quantity, err := strconv.Atoi(quantityStr)
			if err == nil && quantity > 0 {
				filter := bson.M{"username": u.Username, "items.name": itemName}
				update := bson.M{"$set": bson.M{"items.$.quantity": quantity}}
				_, err := shopcollection.UpdateOne(ctx, filter, update)
				if err != nil {
					log.Printf("Error buying: %v", err)
				}
			}
		}

		if itemName := req.FormValue("delete"); itemName != "" {
			filter := bson.M{"username": u.Username}
			update := bson.M{"$pull": bson.M{"items": bson.M{"name": itemName}}}
			_, err := shopcollection.UpdateOne(ctx, filter, update)
			if err != nil {
				log.Printf("Error buying: %v", err)
			}
		}

		if req.FormValue("buy") == "buyNow" {
			selectedItemsJSON := req.FormValue("selectedItems")
			if selectedItemsJSON != "" {
				var selectedItems []cartItem
				err := json.Unmarshal([]byte(selectedItemsJSON), &selectedItems)
				if err == nil {
					// 🛠 Fill missing Screenshot from uCart
					for i := range selectedItems {
						for _, cartItem := range uCart.CartItems {
							if selectedItems[i].Name == cartItem.Name {
								selectedItems[i].Screenshot = cartItem.Screenshot
								break
							}
						}
					}

					totalCost := 0
					for _, item := range selectedItems {
						totalCost += item.Price * item.Quantity
					}

					if uBalance.Balance >= totalCost {
						newBalance := uBalance.Balance - totalCost
						_, err1 := shopcollection1.UpdateOne(ctx, bson.M{"username": u.Username}, bson.M{"$set": bson.M{"balance": newBalance}})
						if err1 == nil {
							for _, item := range selectedItems {
								sellerFilter := bson.M{"username": item.Seller}
								var sellerBalance userBalance
								err := shopcollection1.FindOne(ctx, sellerFilter).Decode(&sellerBalance)
								if err == nil {
									newSellerBalance := sellerBalance.Balance + (item.Price * item.Quantity)
									shopcollection1.UpdateOne(ctx, sellerFilter, bson.M{"$set": bson.M{"balance": newSellerBalance}})
								}
							}
							itemNames := make([]string, 0, len(selectedItems))
							for _, item := range selectedItems {
								itemNames = append(itemNames, item.Name)
							}
							shopcollection.UpdateOne(ctx, bson.M{"username": u.Username}, bson.M{
								"$pull": bson.M{"items": bson.M{"name": bson.M{"$in": itemNames}}},
							})
							successMessage = "Check Out successfully!"
							historyCollection := database.Collection("Histories")

							purchaseRecord := bson.M{
								"username":    u.Username,
								"items":       selectedItems,
								"total":       totalCost,
								"purchasedAt": time.Now(),
							}
							log.Println("success")
							_, err := historyCollection.InsertOne(ctx, purchaseRecord)
							if err != nil {
								log.Println("Failed to record purchase history:", err)
							}
						}
					} else {
						http.Error(res, "Insufficient funds", http.StatusBadRequest)
						return
					}
				}
			}
		}

		// Refresh data after operations
		uCart = getUserWithCart(res, req)
		uBalance = getUserWithBalance(res, req)
	}

	templateinput := struct {
		User           user
		UserBalances   userBalance
		CartItems      []cartItem
		SuccessMessage string
		UserCart       userCart
	}{
		User:           u,
		UserBalances:   uBalance,
		CartItems:      uCart.CartItems,
		SuccessMessage: successMessage,
		UserCart:       uCart,
	}

	tpl.ExecuteTemplate(res, "addToCart.html", templateinput)
}

func viewListing(res http.ResponseWriter, req *http.Request) {
	u := getUser(res, req)
	uBalance := getUserWithBalance(res, req)
	uCart := getUserWithCart(res, req)

	// Get current page
	pageStr := req.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	const pageSize = 20

	// Check if any filters are applied
	filtered := req.FormValue("searchKey") != "" || req.FormValue("categoryKey") != "" || req.FormValue("minprice") != "" || req.FormValue("maxprice") != ""

	var records []item
	var totalItems int
	if filtered {
		records, totalItems = SearchRecords(res, req, page, pageSize)
	} else {
		records, totalItems = DisplayAllRecords(res, req, page, pageSize)
	}

	totalPages := int(math.Ceil(float64(totalItems) / float64(pageSize)))

	templateinput := struct {
		User         user
		Records      []item
		UserBalances userBalance
		UserCart     userCart
		Page         int
		TotalPage    int
		SearchKey    string
		CategoryKey  string
		MinPrice     string
		MaxPrice     string
	}{
		User:         u,
		Records:      records,
		UserBalances: uBalance,
		UserCart:     uCart,
		Page:         page,
		TotalPage:    totalPages,
		SearchKey:    req.FormValue("searchKey"),
		CategoryKey:  req.FormValue("categoryKey"),
		MinPrice:     req.FormValue("minprice"),
		MaxPrice:     req.FormValue("maxprice"),
	}

	tpl.ExecuteTemplate(res, "viewListing.html", templateinput)
}

func detailListing(res http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost {
		action := req.FormValue("action")
		switch action {
		case "Search":
			//get form values
			key := req.FormValue("searchKey")
			http.Redirect(res, req, "/viewListing?searchKey="+url.QueryEscape(key)+"&minprice=&maxprice=&categoryKey=", http.StatusSeeOther)
			return
		case "buyNow", "addtoCart":
			if AlreadyLoggedin(req) {
				u := getUser(res, req)
				record := Handler(res, req)
				uBalance := getUserWithBalance(res, req)
				uCart := getUserWithCart(res, req)

				ctx := req.Context()
				database := ConnectDB(ctx).Database("shoppeeDB")
				balances := database.Collection("Balances")
				cartCollection := database.Collection("Carts")

				username := u.Username
				quantityStr := req.FormValue("quantity")
				quantity, err := strconv.Atoi(quantityStr)
				if err != nil || quantity <= 0 {
					log.Println("Invalid quantity value")
					http.Error(res, "Invalid quantity value", http.StatusBadRequest)
					return
				}

				if action == "buyNow" {
					// BUY NOW FLOW
					total := record.Price * quantity
					if uBalance.Balance < total {
						http.Error(res, "Insufficient balance", http.StatusBadRequest)
						return
					}

					// Deduct buyer's balance
					_, err := balances.UpdateOne(ctx, bson.M{"username": u.Username}, bson.M{
						"$inc": bson.M{"balance": -total},
					})
					if err != nil {
						http.Error(res, "Failed to deduct balance", http.StatusInternalServerError)
						return
					}

					// Credit seller's balance
					_, err = balances.UpdateOne(ctx, bson.M{"username": record.Seller}, bson.M{
						"$inc": bson.M{"balance": total},
					})
					if err != nil {
						http.Error(res, "Failed to credit seller", http.StatusInternalServerError)
						return
					}

					log.Println("Buy now success")
					successMessage := "Purchase successful!"
					historyCollection := database.Collection("Histories")

					purchaseRecord := bson.M{
						"username": u.Username,
						"items": []bson.M{
							{
								"name":     record.Name,
								"price":    record.Price,
								"quantity": quantity,
								"seller":   record.Seller,
							},
						},
						"total":       total,
						"purchasedAt": time.Now(),
					}

					_, err = historyCollection.InsertOne(ctx, purchaseRecord)
					if err != nil {
						log.Println("Failed to record purchase history:", err)
					}

					uBalance = getUserWithBalance(res, req) // refresh balance

					templateinput := struct {
						User           user
						Record         item
						UserBalances   userBalance
						UserCart       userCart
						SuccessMessage string
					}{
						User:           u,
						Record:         record,
						UserBalances:   uBalance,
						UserCart:       uCart,
						SuccessMessage: successMessage,
					}
					tpl.ExecuteTemplate(res, "detailListing.html", templateinput)
					return
				} else {
					// ADD TO CART FLOW
					filter := bson.M{"username": username, "items.screenshot": record.Screenshot}
					update := bson.M{
						"$inc": bson.M{"items.$.quantity": quantity},
					}

					result, err := cartCollection.UpdateOne(ctx, filter, update)
					if err != nil {
						log.Printf("Error updating cart: %v", err)
						http.Error(res, "Failed to update cart", http.StatusInternalServerError)
						return
					}

					if result.MatchedCount == 0 {
						filter = bson.M{"username": username}
						update = bson.M{
							"$setOnInsert": bson.M{"username": username},
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
						UserCart       userCart
						SuccessMessage string
					}{
						User:           u,
						Record:         record,
						UserBalances:   uBalance,
						UserCart:       uCart,
						SuccessMessage: successMessage,
					}

					tpl.ExecuteTemplate(res, "detailListing.html", templateinput)
					return
				}
			} else {
				http.Redirect(res, req, "/login", http.StatusSeeOther)
				return
			}

		default:
			http.Error(res, "Invalid form submission", http.StatusBadRequest)
			return
		}
	}

	// GET method handler — load the item normally
	u := getUser(res, req)
	record := Handler(res, req)
	uBalance := getUserWithBalance(res, req)
	uCart := getUserWithCart(res, req)

	templateinput := struct {
		User         user
		Record       item
		UserBalances userBalance
		UserCart     userCart
	}{
		User:         u,
		Record:       record,
		UserBalances: uBalance,
		UserCart:     uCart,
	}
	tpl.ExecuteTemplate(res, "detailListing.html", templateinput)
}

func historyBuy(res http.ResponseWriter, req *http.Request) {
	if !AlreadyLoggedin(req) {
		http.Redirect(res, req, "/", http.StatusSeeOther)
		return
	}

	ctx := req.Context()
	database := ConnectDB(ctx).Database("shoppeeDB")
	historyCollection := database.Collection("Histories")
	userCollection := database.Collection("Users")

	u := getUser(res, req)
	uBalance := getUserWithBalance(res, req)
	uCart := getUserWithCart(res, req)

	message := ""

	if req.Method == http.MethodPost {
		if req.FormValue("searchKey") != "" {
			key := req.FormValue("searchKey")
			http.Redirect(res, req, "/viewListing?searchKey="+url.QueryEscape(key)+"&minprice=&maxprice=&categoryKey=", http.StatusSeeOther)
			return
		}

		oldPassword := req.FormValue("oldPassword")
		newPassword := req.FormValue("password")
		newUsername := req.FormValue("name")
		log.Println(oldPassword)
		log.Println(newPassword)
		log.Println(newUsername)
		// Find user
		var foundUser struct {
			Username string `bson:"username"`
			Password string `bson:"password"`
		}
		log.Println("start")

		err := userCollection.FindOne(ctx, bson.M{"username": u.Username}).Decode(&foundUser)
		if err != nil {
			log.Println("User not found:", err)
			http.Error(res, "User not found", http.StatusInternalServerError)
			return
		}

		// Check old password
		if foundUser.Password != oldPassword {
			log.Println("Old password is incorrect.", err)
			message = "Old password is incorrect."

		} else {
			// Update in MongoDB
			update := bson.M{
				"$set": bson.M{
					"password": newPassword,
					"username": newUsername,
				},
			}
			log.Println("end")
			_, err := userCollection.UpdateOne(ctx, bson.M{"username": u.Username}, update)
			if err != nil {
				log.Println("Failed to update user:", err)
				message = "Failed to update account."
			} else {
				message = "Account updated successfully."

				// Update in-memory dbUsers and dbSessions
				cookie, err := req.Cookie("session")
				if err == nil {
					sessionID := cookie.Value
					oldUsername := dbSessions[sessionID]

					// Update dbUsers map
					userData := dbUsers[oldUsername]
					delete(dbUsers, oldUsername) // remove old username
					userData.Username = newUsername
					userData.Password = newPassword
					dbUsers[newUsername] = userData

					// Update dbSessions map
					dbSessions[sessionID] = newUsername
				}
			}
		}
	}

	log.Println("Start loading purchase history...")

	type historyDocument struct {
		Username    string     `bson:"username"`
		Items       []cartItem `bson:"items"`
		Total       int        `bson:"total"`
		PurchasedAt time.Time  `bson:"purchasedAt"`
	}

	cursor, err := historyCollection.Find(ctx, bson.M{"username": u.Username})
	if err != nil {
		log.Println("Error finding history:", err)
		http.Error(res, "Failed to load history", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var historyDocs []historyDocument
	if err := cursor.All(ctx, &historyDocs); err != nil {
		log.Println("Error decoding history:", err)
		http.Error(res, "Failed to decode history", http.StatusInternalServerError)
		return
	}

	var historiesItems []cartItem
	for _, doc := range historyDocs {
		for _, item := range doc.Items {
			log.Printf("Purchased item: %s (Screenshot: %s, Quantity: %d)", item.Name, item.Screenshot, item.Quantity)
			historiesItems = append(historiesItems, item)
		}
	}

	templateInput := struct {
		User           user
		UserBalances   userBalance
		UserCart       userCart
		HistoriesItems []cartItem
		Message        string
	}{
		User:           u,
		UserBalances:   uBalance,
		UserCart:       uCart,
		HistoriesItems: historiesItems,
		Message:        message,
	}

	err = tpl.ExecuteTemplate(res, "historyBuy.html", templateInput)
	if err != nil {
		log.Println("Template execution error:", err)
		http.Error(res, "Failed to render page", http.StatusInternalServerError)
	}
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

func DisplayAllRecords(res http.ResponseWriter, req *http.Request, page int, pageSize int) ([]item, int) {
	ctx := req.Context()
	database := ConnectDB(ctx).Database("shoppeeDB")
	shopcollection := database.Collection("Items")

	// Get total count
	totalCount, err := shopcollection.CountDocuments(ctx, bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}

	// Set pagination options
	opts := options.Find()
	opts.SetSkip(int64((page - 1) * pageSize))
	opts.SetLimit(int64(pageSize))

	cursor, err := shopcollection.Find(ctx, bson.D{{}}, opts)
	if err != nil {
		log.Fatal(err)
	}

	var results []item
	if err = cursor.All(ctx, &results); err != nil {
		log.Fatal(err)
	}

	return results, int(totalCount)
}

func getTotalItems() int64 {
	ctx := context.TODO()
	database := ConnectDB(ctx).Database("shoppeeDB")
	shopcollection := database.Collection("Items")

	count, err := shopcollection.CountDocuments(ctx, bson.D{{}})
	if err != nil {
		log.Fatal(err)
	}
	return count
}

func SearchRecords(res http.ResponseWriter, req *http.Request, page int, pageSize int) ([]item, int) {
	ctx := req.Context()
	database := ConnectDB(ctx).Database("shoppeeDB")
	shopcollection := database.Collection("Items")

	var filters bson.D

	// Search bar
	searchKey := req.FormValue("searchKey")
	if searchKey != "" {
		filters = append(filters, bson.E{Key: "$text", Value: bson.D{{Key: "$search", Value: searchKey}}})
	}

	// Category
	categoryKey := req.FormValue("categoryKey")
	if categoryKey != "" {
		filters = append(filters, bson.E{Key: "category", Value: categoryKey})
	}

	// Price
	minPriceStr := req.FormValue("minprice")
	maxPriceStr := req.FormValue("maxprice")
	priceFilter := bson.D{}

	if minPriceStr != "" {
		if minPrice, err := strconv.Atoi(minPriceStr); err == nil {
			priceFilter = append(priceFilter, bson.E{Key: "$gte", Value: minPrice})
		}
	}

	if maxPriceStr != "" {
		if maxPrice, err := strconv.Atoi(maxPriceStr); err == nil {
			priceFilter = append(priceFilter, bson.E{Key: "$lte", Value: maxPrice})
		}
	}

	if len(priceFilter) > 0 {
		filters = append(filters, bson.E{Key: "price", Value: priceFilter})
	}

	// Count total matching items
	totalCount, err := shopcollection.CountDocuments(ctx, bson.D(filters))
	if err != nil {
		log.Printf("Failed to count documents: %v", err)
		return nil, 0
	}

	// Add skip & limit for pagination
	opts := options.Find()
	opts.SetSkip(int64((page - 1) * pageSize))
	opts.SetLimit(int64(pageSize))

	// Final find query
	cursor, err := shopcollection.Find(ctx, bson.D(filters), opts)
	if err != nil {
		log.Printf("Failed to query database: %v", err)
		return nil, 0
	}

	var results []item
	if err := cursor.All(ctx, &results); err != nil {
		log.Printf("Failed to decode results: %v", err)
		return nil, 0
	}

	return results, int(totalCount)
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
	u := getUser(res, req)
	if u.Username == "" {
		return userCart{}
	}

	ctx := req.Context()
	database := ConnectDB(ctx).Database("shoppeeDB")
	shopcollection := database.Collection("Carts")

	filter := bson.D{{"username", u.Username}}
	var result userCart
	err := shopcollection.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		log.Println("No cart found or error decoding:", err)
		return userCart{}
	}

	// Count total items in cart
	totalItems := 0
	for _, item := range result.CartItems {
		totalItems += item.Quantity
	}

	// Add that count to the result
	result.TotalItems = totalItems

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
