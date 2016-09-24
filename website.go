package main

import (
	"github.com/jmoiron/sqlx"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"io"
	"strings"
)

const (
	Driver       = "sqlite3"
	SQLiteFile   = "sqlite.db"
	SQLFileDir   = "sql/"
	DummyFileDir = "dummy/"
)

var (
	db   *sqlx.DB
	stmt = map[string]*sqlx.Stmt{}
)

func init() {
	InitializeDB()
	Init()
	Stmt(`SELECT * FROM articles AS a JOIN users AS u ON a.user_id = u.id LIMIT ? OFFSET ?`)
}

func InitializeDB() {
	var err error
	if Driver == "mysql" {
		db, err = sqlx.Open("mysql", "hoge")
	} else {
		dbFile := SQLiteFile
		db, err = sqlx.Open("sqlite3", dbFile)
	}
	if err != nil {
		log.Fatalf("db connect error: ", err.Error())
	}
}

func execSQLFile(file string) {
	b, err := ioutil.ReadFile(SQLFileDir + file)
	if err != nil {
		log.Fatalf(err.Error())
	}

	_, err = db.Exec(string(b))
	if err != nil {
		log.Fatal(err.Error())
	}
}

func Stmt(query string) *sqlx.Stmt {
	if stmt[query] == nil {
		s, err := db.Preparex(query)
		if err != nil {
			log.Printf("Prepared Statement error: ", err.Error())
		}
		stmt[query] = s
	}
	return stmt[query]
}

func getDummy(file string, obj interface{}) error {
	fh, err := os.Open(DummyFileDir + file)
	if err != nil {
		log.Fatalf(err.Error())
	}

	d := json.NewDecoder(fh)
	return d.Decode(obj)
}

func main() {
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/initialize", initialize)
	http.ListenAndServe(":8000", mux)
}

func initialize(w http.ResponseWriter, r *http.Request) {
	Init()
}

func Init() {
	execSQLFile("create.sql")
	var (
		users      = []User{}
		articles   = []Article{}
		stars      = []Star{}
		setUser    = Stmt(`INSERT INTO users VALUES(?, ?)`)
		setArticle = Stmt(`INSERT INTO articles VALUES(?, ?, ?, ?)`)
		setStar    = Stmt(`INSERT INTO stars VALUES(?, ?, ?)`)
	)

	getDummy("users.json", &users)
	for _, u := range users {
		setUser.Exec(u.ID, u.Name)
	}
	log.Println("users set.")

	getDummy("articles.json", &articles)
	for _, a := range articles {
		setArticle.Exec(a.ID, a.Title, a.UserID, a.Content)
	}
	log.Println("articles set.")

	getDummy("stars.json", &stars)
	for _, s := range stars {
		setStar.Exec(s.ID, s.ArticleID, s.UserID)
	}
	log.Println("stars set.")

}

func index(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.FormValue("l"))
	offset, _ := strconv.Atoi(r.FormValue("o"))
	as := GetArticles(limit, offset)

	for _, a := range as {
		io.WriteString(w,
			"<div>"+
				"<h2>"+ strconv.Itoa(a.ID) + " タイトル: "+ a.Title+ "</h2>"+
				"<div>著者: "+ a.User.Name+ "</div>"+
				"<p>内容: "+
					a.Content+
				"</p>"+
				"<div>ファボ: "+
					func() string {
						var stars []string
						for _, s := range a.Stars {
							stars = append(stars, s.User.Name)
						}
						return strings.Join(stars, ", ")
					}()+
				"</div>"+
			"</div>")
	}
}

func GetArticles(limit, offset int) []Article {
	var (
		articles = []Article{}
	)
	//AS a JOIN users AS u ON a.user_id = u.id
	getArticles := Stmt(`SELECT * FROM articles AS a JOIN users AS u ON a.user_id=u.id LIMIT ? OFFSET ?`)

	r, err := getArticles.Query(limit, offset)
	if err != nil {
		log.Printf(err.Error())
	}
	for r.Next() {
		var a = Article{}
		r.Scan(&a.ID, &a.Title, &a.UserID, &a.Content, &a.User.ID, &a.User.Name)
		GetStars(&a)
		articles = append(articles, a)
	}

	return articles
}

func GetStars(a *Article) {
	starsStmt := Stmt(`SELECT * FROM stars AS s JOIN users AS u ON s.user_id=u.id WHERE article_id=?`)
	r, err := starsStmt.Query(a.ID)
	if err != nil {
		log.Printf(err.Error())
	}
	for r.Next() {
		var s = Star{}
		r.Scan(&s.ID, &s.ArticleID, &s.UserID, &s.User.ID, &s.User.Name)
		a.Stars = append(a.Stars, s)
	}
}

type User struct {
	ID   int    `json:"id" db:"id"`
	Name string `json:"name" db:"name"`
	Articles []Article
	Stars []Star
}

type Article struct {
	ID      int    `json:"id" db:"id"`
	Title   string `json:"title" db:"title"`
	Content string `json:"content" db:"content"`
	UserID  int    `json:"user_id" db:"user_id"`
	User
	Stars []Star
}

type Star struct {
	ID        int `json:"id" db:"id"`
	ArticleID int `json:"article_id" db:"article_id"`
	UserID    int `json:"user_id" db:"user_id"`
	User
	Article
}
