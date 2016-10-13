package main

import (
	"./myutil"
	"encoding/gob"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const (
	ServiceName  = "website"
	SQLFileDir   = "sql/"
	DummyFileDir = "dummy/"
)

//レスポンス
type Response struct {
	Articles []Article
	User     User
	Session  *sessions.Session
}

//モデル定義
type User struct {
	ID         int    `json:"id" db:"id"`
	Name       string `json:"name" db:"name"`
	Articles   []Article
	Favorites  []Favorite
	Followings []User
	Followers  []User
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
	User      User
	Article   Article
}

type Following struct {
	ID     int `json:"id" db:"id"`
	FromID int `json:"from_id" db:"from_id"`
	ToID   int `json:"to_id" db:"to_id"`
	From   User
	To     User
}

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
		"title": func() string {
			return ServiceName
		},
		"showFavs": func(f []Favorite) string {
			var favorites []string
			for _, s := range f {
				favorites = append(favorites, s.User.Name)
			}
			return strings.Join(favorites, ", ")
		},
		"getToken": func(s *sessions.Session) string {
			return s.Values["token"].(string)
		},
		"getUser": func(s *sessions.Session) User {
			return s.Values["user"].(User)
		},
	}).ParseGlob("templates/*.html"))
)

func main() {
	gob.Register(User{})

	var err error
	db, err = sqlx.Open("mysql", "root:root@tcp(192.168.99.100:32773)/go_practice")
	db.SetMaxOpenConns(10)
	if err != nil {
		log.Fatalf("db connect error: ", err.Error())
	}
	defer db.Close()

	mux := http.NewServeMux()

	//静的ファイル
	fileServer := http.FileServer(http.Dir("static"))
	mux.Handle("/js/", fileServer)
	mux.Handle("/css/", fileServer)

	mux.HandleFunc("/", GPMux(index, nil))
	mux.HandleFunc("/user/", GPMux(user, nil))
	mux.HandleFunc("/article", GPMux(nil, postArticle))
	mux.HandleFunc("/login", GPMux(getLogin, postLogin))
	mux.HandleFunc("/logout", GPMux(getLogout, nil))
	mux.HandleFunc("/initialize", GPMux(initialize, nil))
	mux.HandleFunc("/favicon.ico", http.NotFound)

	if err := http.ListenAndServe(":8000", mux); err != nil {
		log.Fatalf(err.Error())
	}
}

func httpError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

type myHandlerFunc func(http.ResponseWriter, *http.Request, *sessions.Session)

func GPMux(getHandler, postHandler myHandlerFunc) http.HandlerFunc {
	//Get, Post Multi Plexer
	return func(w http.ResponseWriter, r *http.Request) {
		var handler myHandlerFunc

		switch r.Method {
		case http.MethodGet:
			handler = getHandler
		case http.MethodPost:
			handler = postHandler
		}
		if handler == nil {
			httpError(w, http.StatusMethodNotAllowed)
			return
		}

		s, err := StartSession(w, r)
		if err != nil {
			return
		}
		handler(w, r, s)
	}
}

func index(w http.ResponseWriter, r *http.Request, s *sessions.Session) {
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
		Session:  s,
	})
	if err != nil {
		log.Printf(err.Error())
	}
}

func user(w http.ResponseWriter, r *http.Request, s *sessions.Session) {
	path := strings.Split(r.RequestURI, "/")
	if len(path) < 3 {
		http.NotFound(w, r)
		return
	}
	userID, err := strconv.Atoi(path[2])
	if err != nil {
		http.NotFound(w, r)
		return
	}

	u := GetUser(userID)
	GetFollowings(&u)
	GetFollowers(&u)

	tpl.ExecuteTemplate(w, "user", Response{
		User:    u,
		Session: s,
	})
}

func postArticle(w http.ResponseWriter, r *http.Request, s *sessions.Session) {

}

func getLogin(w http.ResponseWriter, r *http.Request, s *sessions.Session) {
	tpl.ExecuteTemplate(w, "login", Response{
		Session: s,
	})
}

func postLogin(w http.ResponseWriter, r *http.Request, s *sessions.Session) {
	if u, ok := s.Values["user"].(User); ok {
		if u.Name != "" {
			//ログイン済みならリダイレクト
			http.Redirect(w, r, "/", http.StatusPermanentRedirect)
			return
		}
	} else {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		log.Printf("error! session value: User is null")
		return
	}

	name := r.PostFormValue("name")
	pass := r.PostFormValue("password")
	if name == "" || pass == "" {
		http.Error(w, "名前かパスワードが空です", http.StatusBadRequest)
		return
	}

	getUser := stmt(`SELECT * FROM users WHERE name=?`)
	row, err := getUser.Query(name)
	if err != nil || !row.Next() {
		http.Error(w, "名前かパスワードが間違っています", http.StatusBadRequest)
		return
	}
	var u User
	row.Scan(&u.ID, &u.Name)
	row.Close()

	s.Values["user"] = u
	if err := s.Save(r, w); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}

func getLogout(w http.ResponseWriter, r *http.Request, s *sessions.Session) {
	if token, ok := s.Values["token"]; ok {
		if token == r.FormValue("token") {
			s.Options.MaxAge = -1
			s.Save(r, w)
		}
	}
	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}

func initialize(w http.ResponseWriter, r *http.Request, s *sessions.Session) {
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

	ReadJson("users.json", &users)
	for _, u := range users {
		setUser.Exec(u.Name)
	}
	log.Println("users set.")

	ReadJson("articles.json", &articles)
	for _, a := range articles {
		setArticle.Exec(a.Title, a.UserID, a.Content)
	}
	log.Println("articles set.")

	ReadJson("favorites.json", &favorites)
	for _, s := range favorites {
		setFavorite.Exec(s.ArticleID, s.UserID)
	}
	log.Println("favs set.")

	var fs = [100][100]int{{}}
	for i := 0; i < 1000; i++ {
		fs[rand.Intn(99)][rand.Intn(99)] = 1
	}
	for i, v := range fs {
		for j, h := range v {
			if h == 1 {
				setFollowing.Exec(i+1, j+1)
			}
		}
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

func ReadJson(file string, obj interface{}) error {
	fh, err := os.Open(DummyFileDir + file)
	if err != nil {
		log.Fatalf(err.Error())
	}
	defer fh.Close()

	d := json.NewDecoder(fh)
	return d.Decode(obj)
}

//DB
func GetFollowings(u *User) {
	var followings []User
	r, err := stmt(`SELECT * FROM followings WHERE from_id=?`).Query(u.ID)
	if err != nil {
		log.Print(err.Error())
		return
	}
	for r.Next() {
		var f Following
		r.Scan(&f.ID, &f.FromID, &f.ToID)
		followings = append(followings, GetUser(f.ToID))
	}
	u.Followings = followings
}

func GetFollowers(u *User) {
	var followers []User
	r, err := stmt(`SELECT * FROM followings WHERE to_id=?`).Query(u.ID)
	if err != nil {
		log.Print(err.Error())
		return
	}
	for r.Next() {
		f := Following{}
		r.Scan(&f.ID, &f.FromID, &f.ToID)
		followers = append(followers, GetUser(f.FromID))
	}
	u.Followers = followers
}

func GetArticles(limit, offset int) []Article {
	articles := []Article{}
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
	u := User{}
	row := stmt(`SELECT * FROM users WHERE id=?`).QueryRow(id)
	if err := row.Scan(&u.ID, &u.Name); err != nil {
		return u
	}

	rows, err := stmt(`SELECT * FROM articles WHERE user_id=?`).Query(u.ID)
	if err != nil {
		return u
	}

	for rows.Next() {
		a := Article{}
		rows.Scan(&a.ID, &a.Title, &a.UserID, &a.Content)
		u.Articles = append(u.Articles, a)
	}
	return u
}

func StartSession(w http.ResponseWriter, r *http.Request) (*sessions.Session, error) {
	s, err := store.Get(r, ServiceName)
	if err != nil {
		if pathErr, ok := err.(*os.PathError); ok && pathErr.Err == syscall.Errno(0x2) {
			http.SetCookie(w, &http.Cookie{
				Name:   ServiceName,
				Value:  "",
				MaxAge: -1,
			})
			http.Redirect(w, r, "/", http.StatusMovedPermanently)
		} else {
			httpError(w, http.StatusInternalServerError)
		}
		return s, err
	}
	if s.IsNew {
		s.Values["user"] = User{}
		s.Values["token"] = myutil.RandStr(32)
		err = s.Save(r, w)
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError)
	}
	return s, err
}
