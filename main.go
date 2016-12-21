package main

import (
	//"crypto/tls"
	//"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/russross/blackfriday"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DBHost  = "127.0.0.1"
	DBPort  = ":5432"
	DBUser  = "rebecca"
	DBPass  = "murdoch33111264"
	DBDbase = "d11sa237rihbu6"
)

var (
	database *sql.DB
)

type Comment struct {
	Id          int
	Name        string
	Email       string
	CommentText string
}

type Page struct {
	Id         int
	Title      string
	RawContent string
	Content    template.HTML
	Date       string
	Comments   []Comment
	Session    Session
	GUID       string
}

type JSONResponse struct {
	Fields map[string]string
}

type CommentResp struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Comments string `json:"comments"`
}

type User struct {
	Id   int
	Name string
}

type Session struct {
	Id              string
	Authenticated   bool
	Unauthenticated bool
	User            User
}

func ServeDynamic(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "405: Method Not Allowed", http.StatusMethodNotAllowed)
	} else {
		response := "The time is now " + time.Now().String()
		fmt.Fprint(w, response)
		//renderTemplate(w, response)
	}

}

func ServeStatic(w http.ResponseWriter, r *http.Request) {

	http.ServeFile(w, r, "views/static.html")

}

func pageHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pageID := vars["id"]
	fileName := "files/" + pageID + ".html"
	_, err := os.Stat(fileName)
	if err != nil {
		fileName = "files/404.html"
	}
	http.ServeFile(w, r, fileName)
}

func ServePage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pageGUID := vars["guid"]
	thisPage := Page{}
	fmt.Println(pageGUID)

	err := database.QueryRow("SELECT page_title, page_content, page_date FROM pages WHERE page_guid=$1", pageGUID).Scan(&thisPage.Title, &thisPage.RawContent, &thisPage.Date)
	thisPage.Content = template.HTML(thisPage.RawContent)

	if err != nil {
		log.Println("Couldn't get page: +pageID")
		log.Println(err.Error())
	}

	comments, err := database.Query("SELECT id, comment_name, comment_email, comment_text FROM comments WHERE page_id=$1", pageGUID)
	log.Println(comments)
	if err != nil {
		log.Println(err)
	}
	for comments.Next() {
		var comment Comment
		comments.Scan(&comment.Id, &comment.Name, &comment.Email, &comment.CommentText)
		log.Println(&comment.Id, &comment.Name, &comment.Email, &comment.CommentText)
		thisPage.Comments = append(thisPage.Comments, comment)
		log.Println(thisPage.Comments)
	}
	//html := `<html><head><title>` + thisPage.Title + `</title></head><body><h1>` + thisPage.Title + `</h1><div>` + thisPage.Content + `</div></body></html>`
	//fmt.Fprintln(w, html)
	t, err := template.ParseFiles("templates/blog.html")
	if err != nil {
		panic(err)
	}
	t.Execute(w, thisPage)
	err = t.Execute(os.Stdout, thisPage)
	if err != nil {
		panic(err)
	}

}

func RedirIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/home", 301)

}

func ServeIndex(w http.ResponseWriter, r *http.Request) {
	var Pages = []Page{}
	pages, err := database.Query("SELECT page_title, page_content,page_date, page_guid FROM pages ORDER BY $1 DESC", "page_date")
	fmt.Println(pages.Columns())
	if err != nil {
		log.Println("Error 1: " + err.Error())
		fmt.Fprintln(w, err.Error())
	}
	defer pages.Close()

	for pages.Next() {

		thisPage := Page{}
		pages.Scan(&thisPage.Title, &thisPage.RawContent, &thisPage.Date, &thisPage.GUID)

		thisPage.Content = template.HTML(thisPage.RawContent)
		//log.Println("Log 2: "+thisPage.Title, thisPage.Content, thisPage.Date, thisPage.GUID)
		Pages = append(Pages, thisPage)
	}

	t, err := template.ParseFiles("templates/index.html")
	if err != nil {
		panic(err)
	}

	t.Execute(w, Pages)

	err = t.Execute(os.Stdout, Pages)
	if err != nil {
		panic(err)
	}
	//t.Execute(os.Stdout, Pages)
}

func (p Page) TruncatedText() string {
	chars := 0
	for i := range p.RawContent {
		chars++
		if chars > 150 {
			return p.RawContent[:i] + "..."
		}
	}
	return p.RawContent
}

func APIPage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pageGUID := vars["guid"]
	thisPage := Page{}
	fmt.Println(pageGUID)
	err := database.QueryRow("SELECT page_title, page_content, page_date FROM pages WHERE page_guid=$1", pageGUID).Scan(&thisPage.Title, &thisPage.RawContent, &thisPage.Date)
	thisPage.Content = template.HTML(thisPage.RawContent)
	if err != nil {
		http.Error(w, http.StatusText(404), http.StatusNotFound)
		log.Println(err.Error())
		return
	}
	APIOutput, err := json.Marshal(thisPage)
	fmt.Println(APIOutput)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println(err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, thisPage)
}

func MarkDownHandler(w http.ResponseWriter, r *http.Request) {
	posts := markDownRender()
	//t, err := template.New("templates/blog.html")
	t, err := template.ParseFiles("templates/blog.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println(err.Error())
	}
	t.Execute(w, posts)
	err = t.Execute(os.Stdout, posts)
	if err != nil {
		panic(err)
	}
}

func markDownRender() []Page {
	a := []Page{}
	files, _ := filepath.Glob("posts/*")
	for _, f := range files {
		file := strings.Replace(f, "posts/", "", -1)
		file = strings.Replace(file, ".md", "", -1)
		fileread, _ := ioutil.ReadFile(f)
		lines := strings.Split(string(fileread), "\n")
		title := string(lines[0])
		date := string(lines[1])
		body := strings.Join(lines[2:], "\n")
		body = string(blackfriday.MarkdownCommon([]byte(body)))

		thisPost := Page{}
		thisPost.Title = title
		thisPost.RawContent = body
		thisPost.Date = date
		thisPost.GUID = "test"
		thisPost.Content = template.HTML(thisPost.RawContent)

		a = append(a, thisPost)
	}
	return a
}

func APIPost(w http.ResponseWriter, r *http.Request) {

	if r.Method != "POST" {
		http.ServeFile(w, r, "blog.html")
		return
	}

	var commentAdded bool
	err := r.ParseForm()
	if err != nil {
		log.Println(err.Error())
	}

	//pageId := 0
	pageGUID := "a-new-blog"
	name := r.FormValue("name")
	email := r.FormValue("email")
	comments := r.FormValue("comments")
	res, err := database.Exec("INSERT INTO comments (page_id, comment_name, comment_email, comment_text) VALUES ($1, $2, $3, $4)", pageGUID, name, email, comments)
	if err != nil {
		http.Error(w, "Server error, unable to post comments", 500)
		log.Println(err.Error())
	}

	id, err := res.LastInsertId()
	if err != nil {
		commentAdded = false
	} else {
		commentAdded = true
	}
	commentAddedBool := strconv.FormatBool(commentAdded)
	var resp JSONResponse
	resp.Fields = make(map[string]string)
	resp.Fields["id"] = string(id)
	resp.Fields["added"] = commentAddedBool
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Println(err.Error())
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, jsonResp)
}

// only for reference to broken code does not work
func APIBadPut(w http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)
	id := vars["id"]
	fmt.Println(id)

	err := r.ParseForm()
	if err != nil {
		log.Println(err.Error())
	}

	dump, err := httputil.DumpRequest(r, true)
	if err != nil {
		log.Println(err.Error())
		return
	}

	fmt.Println(string(dump))

	//id := r.FormValue("id")
	name := r.FormValue("name")
	email := r.FormValue("email")
	comments := r.FormValue("comments")
	res, err := database.Exec("UPDATE comments SET comment_name=$1, comment_email=$2, comment_text=$3 WHERE id=$4", name, email, comments, id)
	fmt.Println(res)
	if err != nil {
		log.Println(err.Error)
	}
	var resp JSONResponse
	resp.Fields = make(map[string]string)
	resp.Fields["id"] = string(id)

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Println(err.Error())
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, jsonResp)
}

func APIPut(w http.ResponseWriter, r *http.Request) {
	log.Println("running....")
	/*
		err := r.ParseForm()
		if err != nil {
			log.Println(err.Error())
		}
			log.Println("Starting handler: ", r.PostForm)
			id := r.FormValue("id")
			name := r.FormValue("name")
			email := r.FormValue("email")
			comments := r.FormValue("comments")
	*/
	/*
		vars := mux.Vars(r)

		dump, err := httputil.DumpRequest(r, true)
		if err != nil {
			log.Println(err.Error())
			return
		}

		fmt.Println(string(dump))
		//fmt.Println(vars)
		fmt.Fprintln(w, vars)
	*/
	formdata := CommentResp{}
	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&formdata)
	if err != nil {
		log.Println("decode error: " + err.Error())
		panic(err.Error())
	}
	defer r.Body.Close()

	log.Println(formdata.Name)

	res, err := database.Exec("UPDATE comments SET comment_name=$1, comment_email=$2, comment_text=$3 WHERE id=$4", formdata.Name, formdata.Email, formdata.Comments, formdata.ID)
	fmt.Println(res)
	if err != nil {
		log.Println(err.Error())
	}

	http.Redirect(w, r, "/page/a-new-blog", 301)
}

/*
func RegisterPOST(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatal(err.Error)
	}
	name := r.FormValue("user_name")
	email := r.FormValue("user_email")
	pass := r.FormValue("user_password")
	pageGUID := r.FormValue("referrer")
	// pass2 := r.FormValue("user_password2")
	gure := regexp.MustCompile("[^A-Za-z0-9]+")
	guid := gure.ReplaceAllString(name, "")
	password := weakPasswordHash(pass)
	res, err := database.Exec("INSERT INTO users SET user_name=?, user_guid=?, user_email=?, user_password=?", name, guid, email, password)
	fmt.Println(res)
	if err != nil {
		fmt.Fprintln(w, err.Error)
	} else {
		http.Redirect(w, r, "/page/"+pageGUID, 301)
	}
}

func weakPasswordHash(password string) []byte {
	hash := sha1.New()
	io.WriteString(hash, password)
	return hash.Sum(nil)
}
*/
func main() {

	dbConn := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", DBUser, DBPass, DBDbase)
	//fmt.Println(dbConn)

	//db, err := sql.Open("mysql", dbConn)

	db, err := sql.Open("postgres", dbConn)
	if err != nil {
		log.Println("Couldn't connect to" + DBDbase)
		log.Println(err.Error())
	}

	defer db.Close()

	database = db

	rtr := mux.NewRouter()

	cssHandler := http.FileServer(http.Dir("./static/css/"))
	imagesHandler := http.FileServer(http.Dir("./static/images/"))
	jsHandler := http.FileServer(http.Dir("./static/js/"))
	rtr.PathPrefix("/static/css").Handler(http.StripPrefix("/static/css", cssHandler))
	rtr.PathPrefix("/static/images").Handler(http.StripPrefix("/static/images", imagesHandler))
	rtr.PathPrefix("/page/static/js").Handler(http.StripPrefix("/page/static/js", jsHandler))
	http.Handle("/static/css/", http.StripPrefix("/static/css/", cssHandler))
	http.Handle("/static/images/", http.StripPrefix("/static/images/", imagesHandler))
	http.Handle("/static/js/", http.StripPrefix("/static/images/", jsHandler))

	rtr.HandleFunc("/page/{guid:[0-9a-zA\\-]+}", ServePage)
	rtr.HandleFunc("/", RedirIndex)
	rtr.HandleFunc("/home", ServeIndex)
	rtr.HandleFunc("/api/pages", APIPage).Methods("GET")
	rtr.HandleFunc("/api/pages/{guid:[0-9a-zA\\-]+}", APIPage).Methods("GET")
	rtr.HandleFunc("/markdown", MarkDownHandler)
	rtr.HandleFunc("/api/comments", APIPost).Methods("POST")
	//rtr.HandleFunc("/api/comments/{id:[\\w\\d\\-]+}", APIPut).Methods("PUT").Schemes("https")
	rtr.HandleFunc("/api/commentz", APIPut).Methods("POST")
	//routes.HandleFunc("/register", RegisterPOST).Methods("POST").Schemes("https")
	//routes.HandleFunc("/login", LoginPOST).Methods("POST").Schemes("https")
	rtr.HandleFunc("/static", ServeStatic)
	http.Handle("/", rtr)

	/*
		log.SetFlags(log.Lshortfile)
		certificates, err := tls.LoadX509KeyPair("server.pem", "server.key")
		if err != nil {
			log.Println(err.Error())
		}

		tlsConf := tls.Config{Certificates: []tls.Certificate{certificates}}

		ln, err := tls.Listen("tcp", ":9000", &tlsConf)
		if err != nil {
			log.Println(err.Error())
			return
		}
		defer ln.Close()
	*/

	//http.HandleFunc("/", ServeDynamic)

	/*
		log.Printf("About to listen on 10443. Go to https://127.0.0.1:10443/")
		zz := http.ListenAndServeTLS(":10443", "server.pem", "server.key", rtr)
		if err != nil {
			log.Println(err.Error())
			log.Fatal("ListenAndServe: ", err)
		}
		log.Fatal(zz)
	*/

	//http.ListenAndServe(":8080", nil)

	http.ListenAndServe(":"+os.Getenv("PORT"), nil)
}
