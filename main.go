package main

import (
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	ServiceName  = "website"
	SQLFileDir   = "sql/"
	DummyFileDir = "dummy/"
)

var (
	db *sqlx.DB

	stmt = func() func(query string) *sqlx.Stmt {
		var stmt = map[string]*sqlx.Stmt{}
		return func(query string) *sqlx.Stmt {
			if stmt[query] == nil {
				s, err := db.Preparex(query)
				if err != nil {
					log.Printf("Prepared Statement error: ", err.Error())
				}
				stmt[query] = s
			}
			return stmt[query]
		}
	}()

	store = sessions.NewFilesystemStore("sess", []byte(ServiceName))

	tpl = template.Must(template.New("tmpl").Funcs(template.FuncMap{
		"showFavs": func(f []Favorite) string {
			var favorites []string
			for _, s := range f {
				favorites = append(favorites, s.User.Name)
			}
			return strings.Join(favorites, ", ")
		},
		"getFollowings": func(u User) []Following {
			return GetFollowings(u)
		},
		"getFollowers": func(u User) []Following {
			return GetFollowers(u)
		},
	}).ParseGlob("templates/*.html"))
)

func main() {
	var err error
	db, err = sqlx.Open("mysql", "root:root@tcp(192.168.99.100:32773)/go_practice")
	db.SetMaxOpenConns(10)
	if err != nil {
		log.Fatalf("db connect error: ", err.Error())
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/user/", user)
	mux.HandleFunc("/initialize", initialize)

	//faviconなし
	mux.HandleFunc("/favicon.ico", http.NotFound)

	http.ListenAndServe(":8000", mux)
}

func index(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.FormValue("l"))
	if limit < 1 {
		limit = 10
	}
	offset, _ := strconv.Atoi(r.FormValue("o"))
	if offset < 1 {
		offset = 0
	}

	as := GetArticles(limit, offset)
	err := tpl.ExecuteTemplate(w, "index", Response{
		Articles: as,
	})
	if err != nil {
		log.Printf(err.Error())
	}
}

func user(w http.ResponseWriter, r *http.Request) {
	path := strings.Split(r.RequestURI, "/")
	if len(path) < 3 {
		http.NotFound(w, r)
	}
	userID, err := strconv.Atoi(path[2])
	if err != nil {
		http.NotFound(w, r)
		return
	}

	tpl.ExecuteTemplate(w, "user", Response{
		User: GetUser(userID),
	})
}

func initialize(w http.ResponseWriter, r *http.Request) {
	ExecSQLFile("create.sql")
	var (
		users        []User
		articles     []Article
		favorites    []Favorite
		setUser      = stmt(`INSERT INTO users (name) VALUES(?)`)
		setArticle   = stmt(`INSERT INTO articles (title, user_id, content) VALUES(?, ?, ?)`)
		setFavorite  = stmt(`INSERT INTO favorites (article_id, user_id) VALUES(?, ?)`)
		setFollowing = stmt(`INSERT INTO followings (from_id, to_id) VALUES(?, ?)`)
	)
	defer func() {
		setUser.Close()
		setArticle.Close()
		setFavorite.Close()
		setFollowing.Close()
	}()

	GetDummy("users.json", &users)
	for _, u := range users {
		setUser.Exec(u.Name)
	}
	log.Println("users set.")

	GetDummy("articles.json", &articles)
	for _, a := range articles {
		setArticle.Exec(a.Title, a.UserID, a.Content)
	}
	log.Println("articles set.")

	GetDummy("favorites.json", &favorites)
	for _, s := range favorites {
		setFavorite.Exec(s.ArticleID, s.UserID)
	}
	log.Println("favs set.")

	for i := 0; i < 10000; i++ {
		setFollowing.Exec(rand.Intn(99)+1, rand.Intn(99)+1)
	}
	log.Println("followings set.")

	io.WriteString(w, "done")
}

func ExecSQLFile(file string) {
	b, err := ioutil.ReadFile(SQLFileDir + file)
	if err != nil {
		log.Fatalf(err.Error())
	}

	for _, q := range strings.Split(string(b), "\n\n") {
		_, err := db.Exec(q)
		if err != nil {
			log.Fatal("exec SQL error: ", err.Error())
		}
	}
}

func GetDummy(file string, obj interface{}) error {
	fh, err := os.Open(DummyFileDir + file)
	if err != nil {
		log.Fatalf(err.Error())
	}

	d := json.NewDecoder(fh)
	return d.Decode(obj)
}

func GetFollowings(u User) []Following {
	var followings []Following
	r, err := stmt(`SELECT * FROM followings WHERE from_id=?`).Query(u.ID)
	if err != nil {
		log.Print(err.Error())
		return followings
	}
	for r.Next() {
		var f Following
		r.Scan(&f.ID, &f.FromID, &f.ToID)
		f.To = GetUser(f.ToID)
		followings = append(followings, f)
	}
	return followings
}

func GetFollowers(u User) []Following {
	var followings []Following
	r, err := stmt(`SELECT * FROM followings WHERE to_id=?`).Query(u.ID)
	if err != nil {
		log.Print(err.Error())
		return followings
	}
	for r.Next() {
		var f Following
		r.Scan(&f.ID, &f.FromID, &f.ToID)
		f.From = GetUser(f.FromID)
		followings = append(followings, f)
	}
	return followings
}

func GetArticles(limit, offset int) []Article {
	var (
		articles = []Article{}
	)
	getArticles := stmt(`SELECT a.*, u.* FROM (SELECT a.id FROM articles AS a ORDER BY a.id DESC LIMIT ? OFFSET ?) AS a1 JOIN articles AS a ON a.id=a1.id JOIN users AS u ON a.user_id=u.id;`)

	r, err := getArticles.Query(limit, offset)
	if err != nil {
		log.Printf(err.Error())
	}
	for r.Next() {
		var a = Article{}
		r.Scan(&a.ID, &a.Title, &a.UserID, &a.Content, &a.User.ID, &a.User.Name)
		GetFavorites(&a)
		articles = append(articles, a)
	}

	return articles
}

func GetFavorites(a *Article) {
	getFavorites := stmt(`SELECT * FROM favorites AS s JOIN users AS u ON s.user_id=u.id WHERE article_id=?`)
	r, err := getFavorites.Query(a.ID)
	if err != nil {
		log.Printf(err.Error())
	}
	for r.Next() {
		var s = Favorite{}
		r.Scan(&s.ID, &s.ArticleID, &s.UserID, &s.User.ID, &s.User.Name)
		a.Favorites = append(a.Favorites, s)
	}
}

func GetUser(id int) User {
	var u User
	row := stmt(`SELECT * FROM users WHERE id=?`).QueryRow(id)
	if err := row.Scan(&u.ID, &u.Name); err != nil {
		return u
	}

	rows, err := stmt(`SELECT * FROM articles WHERE user_id=?`).Query(u.ID)
	if err != nil {
		return u
	}

	for rows.Next() {
		var a Article
		rows.Scan(&a.ID, &a.Title, &a.UserID, &a.Content)
		u.Articles = append(u.Articles, a)
	}
	return u
}

//レスポンス
type Response struct {
	Articles []Article
	User     User
}

//モデル定義
type User struct {
	ID        int    `json:"id" db:"id"`
	Name      string `json:"name" db:"name"`
	Articles  []Article
	Favorites []Favorite
	Following []User
	Follower  []User
}

type Article struct {
	ID        int    `json:"id" db:"id"`
	Title     string `json:"title" db:"title"`
	Content   string `json:"content" db:"content"`
	UserID    int    `json:"user_id" db:"user_id"`
	User      User
	Favorites []Favorite
}

type Favorite struct {
	ID        int `json:"id" db:"id"`
	ArticleID int `json:"article_id" db:"article_id"`
	UserID    int `json:"user_id" db:"user_id"`
	User
	Article
}

type Following struct {
	ID     int `json:"id" db:"id"`
	FromID int `json:"from_id" db:"from_id"`
	ToID   int `json:"to_id" db:"to_id"`
	From   User
	To     User
}
