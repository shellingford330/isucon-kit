package main

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	gsm "github.com/bradleypeabody/gorilla-sessions-memcache"
	cache "github.com/go-redis/cache/v8"
	redis "github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	goji "goji.io"
	"goji.io/pat"
	"goji.io/pattern"
)

var (
	db      *sqlx.DB
	store   *gsm.MemcacheStore
	mycache *cache.Cache
)

const (
	postsPerPage  = 20
	ISO8601Format = "2006-01-02T15:04:05-07:00"
	UploadLimit   = 10 * 1024 * 1024 // 10mb
)

type User struct {
	ID          int       `db:"id"`
	AccountName string    `db:"account_name"`
	Passhash    string    `db:"passhash"`
	Authority   int       `db:"authority"`
	DelFlg      int       `db:"del_flg"`
	CreatedAt   time.Time `db:"created_at"`
}

type Post struct {
	ID           int       `db:"id"`
	UserID       int       `db:"user_id"`
	Imgdata      []byte    `db:"imgdata"`
	Body         string    `db:"body"`
	Mime         string    `db:"mime"`
	CreatedAt    time.Time `db:"created_at"`
	CommentCount int
	Comments     []Comment
	User         User
	CSRFToken    string
}

type Comment struct {
	ID        int       `db:"id"`
	PostID    int       `db:"post_id"`
	UserID    int       `db:"user_id"`
	Comment   string    `db:"comment"`
	CreatedAt time.Time `db:"created_at"`
	User      User
}

func init() {
	memdAddr := os.Getenv("ISUCONP_MEMCACHED_ADDRESS")
	if memdAddr == "" {
		memdAddr = "localhost:11211"
	}
	memcacheClient := memcache.New(memdAddr)
	store = gsm.NewMemcacheStore(memcacheClient, "iscogram_", []byte("sendagaya"))
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

func dbInitialize() {
	sqls := []string{
		"DELETE FROM users WHERE id > 1000",
		"DELETE FROM posts WHERE id > 10000",
		"DELETE FROM comments WHERE id > 100000",
		"UPDATE users SET del_flg = 0",
		"UPDATE users SET del_flg = 1 WHERE id % 50 = 0",
	}

	for _, sql := range sqls {
		db.Exec(sql)
	}
}

func tryLogin(accountName, password string) *User {
	u := User{}
	err := db.Get(&u, "SELECT * FROM users WHERE account_name = ? AND del_flg = 0", accountName)
	if err != nil {
		return nil
	}

	if calculatePasshash(u.AccountName, password) == u.Passhash {
		return &u
	} else {
		return nil
	}
}

func validateUser(accountName, password string) bool {
	return regexp.MustCompile(`\A[0-9a-zA-Z_]{3,}\z`).MatchString(accountName) &&
		regexp.MustCompile(`\A[0-9a-zA-Z_]{6,}\z`).MatchString(password)
}

// 今回のGo実装では言語側のエスケープの仕組みが使えないのでOSコマンドインジェクション対策できない
// 取り急ぎPHPのescapeshellarg関数を参考に自前で実装
// cf: http://jp2.php.net/manual/ja/function.escapeshellarg.php
func escapeshellarg(arg string) string {
	return "'" + strings.Replace(arg, "'", "'\\''", -1) + "'"
}

func digest(src string) string {
	// opensslのバージョンによっては (stdin)= というのがつくので取る
	out, err := exec.Command("/bin/bash", "-c", `printf "%s" `+escapeshellarg(src)+` | openssl dgst -sha512 | sed 's/^.*= //'`).Output()
	if err != nil {
		log.Print(err)
		return ""
	}

	return strings.TrimSuffix(string(out), "\n")
}

func calculateSalt(accountName string) string {
	return digest(accountName)
}

func calculatePasshash(accountName, password string) string {
	return digest(password + ":" + calculateSalt(accountName))
}

func getSession(r *http.Request) *sessions.Session {
	session, _ := store.Get(r, "isuconp-go.session")

	return session
}

func getSessionUser(r *http.Request) User {
	session := getSession(r)
	uid, ok := session.Values["user_id"]
	if !ok || uid == nil {
		return User{}
	}

	u := User{}

	err := db.Get(&u, "SELECT * FROM `users` WHERE `id` = ?", uid)
	if err != nil {
		return User{}
	}

	return u
}

func getFlash(w http.ResponseWriter, r *http.Request, key string) string {
	session := getSession(r)
	value, ok := session.Values[key]

	if !ok || value == nil {
		return ""
	} else {
		delete(session.Values, key)
		session.Save(r, w)
		return value.(string)
	}
}

func makePosts(results []Post, csrfToken string, allComments bool) ([]Post, error) {
	var posts []Post

	for _, p := range results {
		var commentCount int
		key := fmt.Sprintf("comment_count:post_id:%d", p.ID)
		err := mycache.Get(context.Background(), key, &commentCount)
		if err != nil && err != cache.ErrCacheMiss {
			log.Print(err)
			return nil, err
		}
		if err == cache.ErrCacheMiss {
			err = db.Get(&commentCount, "SELECT COUNT(*) AS `count` FROM `comments` WHERE `post_id` = ?", p.ID)
			if err != nil {
				log.Print(err)
				return nil, err
			}

			err = mycache.Set(&cache.Item{
				Ctx:   context.Background(),
				Key:   key,
				Value: commentCount,
				TTL:   10 * time.Second,
			})
			if err != nil {
				log.Print(err)
				return nil, err
			}
		}
		p.CommentCount = commentCount

		query := "SELECT c.id AS `id`, c.post_id AS `post_id`, c.user_id AS `user_id`, c.comment AS `comment`, c.created_at AS `created_at`, " +
			"u.id AS `user.id`, u.account_name AS `user.account_name`, u.passhash AS `user.passhash`, u.authority AS `user.authority`, u.del_flg AS `user.del_flg`, u.created_at AS `user.created_at` " +
			"FROM `comments` c " +
			"INNER JOIN `users` u ON u.id = c.user_id " +
			"WHERE c.post_id = ? ORDER BY c.created_at DESC"
		if !allComments {
			query += " LIMIT 3"
		}
		var comments []Comment
		err = db.Select(&comments, query, p.ID)
		if err != nil {
			return nil, err
		}

		// reverse
		for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
			comments[i], comments[j] = comments[j], comments[i]
		}

		p.Comments = comments

		err = db.Get(&p.User, "SELECT * FROM `users` WHERE `id` = ?", p.UserID)
		if err != nil {
			return nil, err
		}

		p.CSRFToken = csrfToken

		if p.User.DelFlg == 0 {
			posts = append(posts, p)
		}
		if len(posts) >= postsPerPage {
			break
		}
	}

	return posts, nil
}

func imageURL(p Post) string {
	ext := ""
	if p.Mime == "image/jpeg" {
		ext = ".jpg"
	} else if p.Mime == "image/png" {
		ext = ".png"
	} else if p.Mime == "image/gif" {
		ext = ".gif"
	}

	return "/image/" + strconv.Itoa(p.ID) + ext
}

func isLogin(u User) bool {
	return u.ID != 0
}

func getCSRFToken(r *http.Request) string {
	session := getSession(r)
	csrfToken, ok := session.Values["csrf_token"]
	if !ok {
		return ""
	}
	return csrfToken.(string)
}

func secureRandomStr(b int) string {
	k := make([]byte, b)
	if _, err := crand.Read(k); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", k)
}

func getTemplPath(filename string) string {
	return path.Join("templates", filename)
}

func getInitialize(w http.ResponseWriter, r *http.Request) {
	dbInitialize()
	w.WriteHeader(http.StatusOK)
}

func getLogin(w http.ResponseWriter, r *http.Request) {
	me := getSessionUser(r)

	if isLogin(me) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	template.Must(template.ParseFiles(
		getTemplPath("layout.html"),
		getTemplPath("login.html")),
	).Execute(w, struct {
		Me    User
		Flash string
	}{me, getFlash(w, r, "notice")})
}

func postLogin(w http.ResponseWriter, r *http.Request) {
	if isLogin(getSessionUser(r)) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	u := tryLogin(r.FormValue("account_name"), r.FormValue("password"))

	if u != nil {
		session := getSession(r)
		session.Values["user_id"] = u.ID
		session.Values["csrf_token"] = secureRandomStr(16)
		session.Save(r, w)

		http.Redirect(w, r, "/", http.StatusFound)
	} else {
		session := getSession(r)
		session.Values["notice"] = "アカウント名かパスワードが間違っています"
		session.Save(r, w)

		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

func getRegister(w http.ResponseWriter, r *http.Request) {
	if isLogin(getSessionUser(r)) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	template.Must(template.ParseFiles(
		getTemplPath("layout.html"),
		getTemplPath("register.html")),
	).Execute(w, struct {
		Me    User
		Flash string
	}{User{}, getFlash(w, r, "notice")})
}

func postRegister(w http.ResponseWriter, r *http.Request) {
	if isLogin(getSessionUser(r)) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	accountName, password := r.FormValue("account_name"), r.FormValue("password")

	validated := validateUser(accountName, password)
	if !validated {
		session := getSession(r)
		session.Values["notice"] = "アカウント名は3文字以上、パスワードは6文字以上である必要があります"
		session.Save(r, w)

		http.Redirect(w, r, "/register", http.StatusFound)
		return
	}

	exists := 0
	// ユーザーが存在しない場合はエラーになるのでエラーチェックはしない
	db.Get(&exists, "SELECT 1 FROM users WHERE `account_name` = ?", accountName)

	if exists == 1 {
		session := getSession(r)
		session.Values["notice"] = "アカウント名がすでに使われています"
		session.Save(r, w)

		http.Redirect(w, r, "/register", http.StatusFound)
		return
	}

	query := "INSERT INTO `users` (`account_name`, `passhash`) VALUES (?,?)"
	result, err := db.Exec(query, accountName, calculatePasshash(accountName, password))
	if err != nil {
		log.Print(err)
		return
	}

	session := getSession(r)
	uid, err := result.LastInsertId()
	if err != nil {
		log.Print(err)
		return
	}
	session.Values["user_id"] = uid
	session.Values["csrf_token"] = secureRandomStr(16)
	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusFound)
}

func getLogout(w http.ResponseWriter, r *http.Request) {
	session := getSession(r)
	delete(session.Values, "user_id")
	session.Options = &sessions.Options{MaxAge: -1}
	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusFound)
}

func getIndex(w http.ResponseWriter, r *http.Request) {
	me := getSessionUser(r)

	results := []Post{}

	query := "SELECT p.id AS `id`, p.user_id AS `user_id`, p.body AS `body`, p.mime AS `mime`, p.created_at AS `created_at`, " +
		"u.id AS `user.id`, u.account_name AS `user.account_name`, u.passhash AS `user.passhash`, u.authority AS `user.authority`, u.del_flg AS `user.del_flg`, u.created_at AS `user.created_at` " +
		"FROM `posts` p " +
		"INNER JOIN `users` u ON u.id = p.user_id " +
		"WHERE u.del_flg = 0 " +
		"ORDER BY p.created_at DESC " +
		"LIMIT ?"
	err := db.Select(&results, query, postsPerPage)
	if err != nil {
		log.Print(err)
		return
	}

	// start
	csrfToken := getCSRFToken(r)

	for i := range results {
		var commentCount int
		key := fmt.Sprintf("comment_count:post_id:%d", results[i].ID)
		err := mycache.Get(context.Background(), key, &commentCount)
		if err != nil && err != cache.ErrCacheMiss {
			log.Print(err)
			return
		}
		if err == cache.ErrCacheMiss {
			err = db.Get(&commentCount, "SELECT COUNT(*) AS `count` FROM `comments` WHERE `post_id` = ?", results[i].ID)
			if err != nil {
				log.Print(err)
				return
			}

			err = mycache.Set(&cache.Item{
				Ctx:   context.Background(),
				Key:   key,
				Value: commentCount,
				TTL:   10 * time.Second,
			})
			if err != nil {
				log.Print(err)
				return
			}
		}
		results[i].CommentCount = commentCount

		var comments []Comment
		key = fmt.Sprintf("comments:post_id:%d", results[i].ID)
		err = mycache.Get(context.Background(), key, &comments)
		if err != nil && err != cache.ErrCacheMiss {
			log.Print(err)
			return
		}
		if err == cache.ErrCacheMiss {
			query := "SELECT c.id AS `id`, c.post_id AS `post_id`, c.user_id AS `user_id`, c.comment AS `comment`, c.created_at AS `created_at`, " +
				"u.id AS `user.id`, u.account_name AS `user.account_name`, u.passhash AS `user.passhash`, u.authority AS `user.authority`, u.del_flg AS `user.del_flg`, u.created_at AS `user.created_at` " +
				"FROM `comments` c " +
				"INNER JOIN `users` u ON u.id = c.user_id " +
				"WHERE c.post_id = ? ORDER BY c.created_at DESC LIMIT 3"
			err = db.Select(&comments, query, results[i].ID)
			if err != nil {
				log.Print(err)
				return
			}

			// reverse
			for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
				comments[i], comments[j] = comments[j], comments[i]
			}

			err = mycache.Set(&cache.Item{
				Ctx:   context.Background(),
				Key:   key,
				Value: comments,
				TTL:   10 * time.Second,
			})
			if err != nil {
				log.Print(err)
				return
			}
		}
		results[i].Comments = comments

		results[i].CSRFToken = csrfToken
	}
	// end

	fmap := template.FuncMap{
		"imageURL": imageURL,
	}

	template.Must(template.New("layout.html").Funcs(fmap).ParseFiles(
		getTemplPath("layout.html"),
		getTemplPath("index.html"),
		getTemplPath("posts.html"),
		getTemplPath("post.html"),
	)).Execute(w, struct {
		Posts     []Post
		Me        User
		CSRFToken string
		Flash     string
	}{results, me, getCSRFToken(r), getFlash(w, r, "notice")})
}

func getAccountName(w http.ResponseWriter, r *http.Request) {
	accountName := pat.Param(r, "accountName")
	user := User{}

	err := db.Get(&user, "SELECT * FROM `users` WHERE `account_name` = ? AND `del_flg` = 0", accountName)
	if err != nil {
		log.Print(err)
		return
	}

	if user.ID == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	results := []Post{}

	query := "SELECT p.id AS `id`, p.user_id AS `user_id`, p.body AS `body`, p.mime AS `mime`, p.created_at AS `created_at`, " +
		"u.id AS `user.id`, u.account_name AS `user.account_name`, u.passhash AS `user.passhash`, u.authority AS `user.authority`, u.del_flg AS `user.del_flg`, u.created_at AS `user.created_at` " +
		"FROM `posts` p " +
		"INNER JOIN `users` u ON u.id = p.user_id " +
		"WHERE p.user_id = ? " +
		"ORDER BY p.created_at DESC " +
		"LIMIT ?"
	err = db.Select(&results, query, user.ID, postsPerPage)
	if err != nil {
		log.Print(err)
		return
	}

	// start
	csrfToken := getCSRFToken(r)

	for i := range results {
		var commentCount int
		key := fmt.Sprintf("comment_count:post_id:%d", results[i].ID)
		err := mycache.Get(context.Background(), key, &commentCount)
		if err != nil && err != cache.ErrCacheMiss {
			log.Print(err)
			return
		}
		if err == cache.ErrCacheMiss {
			err = db.Get(&commentCount, "SELECT COUNT(*) AS `count` FROM `comments` WHERE `post_id` = ?", results[i].ID)
			if err != nil {
				log.Print(err)
				return
			}

			err = mycache.Set(&cache.Item{
				Ctx:   context.Background(),
				Key:   key,
				Value: commentCount,
				TTL:   10 * time.Second,
			})
			if err != nil {
				log.Print(err)
				return
			}
		}
		results[i].CommentCount = commentCount

		var comments []Comment
		key = fmt.Sprintf("comments:post_id:%d", results[i].ID)
		err = mycache.Get(context.Background(), key, &comments)
		if err != nil && err != cache.ErrCacheMiss {
			log.Print(err)
			return
		}
		if err == cache.ErrCacheMiss {
			query := "SELECT c.id AS `id`, c.post_id AS `post_id`, c.user_id AS `user_id`, c.comment AS `comment`, c.created_at AS `created_at`, " +
				"u.id AS `user.id`, u.account_name AS `user.account_name`, u.passhash AS `user.passhash`, u.authority AS `user.authority`, u.del_flg AS `user.del_flg`, u.created_at AS `user.created_at` " +
				"FROM `comments` c " +
				"INNER JOIN `users` u ON u.id = c.user_id " +
				"WHERE c.post_id = ? ORDER BY c.created_at DESC LIMIT 3"
			err = db.Select(&comments, query, results[i].ID)
			if err != nil {
				log.Print(err)
				return
			}

			// reverse
			for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
				comments[i], comments[j] = comments[j], comments[i]
			}

			err = mycache.Set(&cache.Item{
				Ctx:   context.Background(),
				Key:   key,
				Value: comments,
				TTL:   10 * time.Second,
			})
			if err != nil {
				log.Print(err)
				return
			}
		}
		results[i].Comments = comments

		results[i].CSRFToken = csrfToken
	}
	// end

	commentCount := 0
	err = db.Get(&commentCount, "SELECT COUNT(*) AS count FROM `comments` WHERE `user_id` = ?", user.ID)
	if err != nil {
		log.Print(err)
		return
	}

	postIDs := []int{}
	err = db.Select(&postIDs, "SELECT `id` FROM `posts` WHERE `user_id` = ?", user.ID)
	if err != nil {
		log.Print(err)
		return
	}
	postCount := len(postIDs)

	commentedCount := 0
	if postCount > 0 {
		s := []string{}
		for range postIDs {
			s = append(s, "?")
		}
		placeholder := strings.Join(s, ", ")

		// convert []int -> []interface{}
		args := make([]interface{}, len(postIDs))
		for i, v := range postIDs {
			args[i] = v
		}

		err = db.Get(&commentedCount, "SELECT COUNT(*) AS count FROM `comments` WHERE `post_id` IN ("+placeholder+")", args...)
		if err != nil {
			log.Print(err)
			return
		}
	}

	me := getSessionUser(r)

	fmap := template.FuncMap{
		"imageURL": imageURL,
	}

	template.Must(template.New("layout.html").Funcs(fmap).ParseFiles(
		getTemplPath("layout.html"),
		getTemplPath("user.html"),
		getTemplPath("posts.html"),
		getTemplPath("post.html"),
	)).Execute(w, struct {
		Posts          []Post
		User           User
		PostCount      int
		CommentCount   int
		CommentedCount int
		Me             User
	}{results, user, postCount, commentCount, commentedCount, me})
}

func getPosts(w http.ResponseWriter, r *http.Request) {
	m, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Print(err)
		return
	}
	maxCreatedAt := m.Get("max_created_at")
	if maxCreatedAt == "" {
		return
	}

	t, err := time.Parse(ISO8601Format, maxCreatedAt)
	if err != nil {
		log.Print(err)
		return
	}

	results := []Post{}
	query := "SELECT p.id AS `id`, p.user_id AS `user_id`, p.body AS `body`, p.mime AS `mime`, p.created_at AS `created_at`, " +
		"u.id AS `user.id`, u.account_name AS `user.account_name`, u.passhash AS `user.passhash`, u.authority AS `user.authority`, u.del_flg AS `user.del_flg`, u.created_at AS `user.created_at` " +
		"FROM `posts` p " +
		"INNER JOIN `users` u ON u.id = p.user_id " +
		"WHERE u.del_flg = 0 AND p.created_at <= ? " +
		"ORDER BY p.created_at DESC " +
		"LIMIT ?"
	err = db.Select(&results, query, t.Format(ISO8601Format), postsPerPage)
	if err != nil {
		log.Print(err)
		return
	}

	if len(results) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// start
	csrfToken := getCSRFToken(r)

	for i := range results {
		var commentCount int
		key := fmt.Sprintf("comment_count:post_id:%d", results[i].ID)
		err := mycache.Get(context.Background(), key, &commentCount)
		if err != nil && err != cache.ErrCacheMiss {
			log.Print(err)
			return
		}
		if err == cache.ErrCacheMiss {
			err = db.Get(&commentCount, "SELECT COUNT(*) AS `count` FROM `comments` WHERE `post_id` = ?", results[i].ID)
			if err != nil {
				log.Print(err)
				return
			}

			err = mycache.Set(&cache.Item{
				Ctx:   context.Background(),
				Key:   key,
				Value: commentCount,
				TTL:   10 * time.Second,
			})
			if err != nil {
				log.Print(err)
				return
			}
		}
		results[i].CommentCount = commentCount

		var comments []Comment
		key = fmt.Sprintf("comments:post_id:%d", results[i].ID)
		err = mycache.Get(context.Background(), key, &comments)
		if err != nil && err != cache.ErrCacheMiss {
			log.Print(err)
			return
		}
		if err == cache.ErrCacheMiss {
			query := "SELECT c.id AS `id`, c.post_id AS `post_id`, c.user_id AS `user_id`, c.comment AS `comment`, c.created_at AS `created_at`, " +
				"u.id AS `user.id`, u.account_name AS `user.account_name`, u.passhash AS `user.passhash`, u.authority AS `user.authority`, u.del_flg AS `user.del_flg`, u.created_at AS `user.created_at` " +
				"FROM `comments` c " +
				"INNER JOIN `users` u ON u.id = c.user_id " +
				"WHERE c.post_id = ? ORDER BY c.created_at DESC LIMIT 3"
			err = db.Select(&comments, query, results[i].ID)
			if err != nil {
				log.Print(err)
				return
			}

			// reverse
			for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
				comments[i], comments[j] = comments[j], comments[i]
			}

			err = mycache.Set(&cache.Item{
				Ctx:   context.Background(),
				Key:   key,
				Value: comments,
				TTL:   10 * time.Second,
			})
			if err != nil {
				log.Print(err)
				return
			}
		}
		results[i].Comments = comments

		results[i].CSRFToken = csrfToken
	}
	// end

	fmap := template.FuncMap{
		"imageURL": imageURL,
	}

	template.Must(template.New("posts.html").Funcs(fmap).ParseFiles(
		getTemplPath("posts.html"),
		getTemplPath("post.html"),
	)).Execute(w, results)
}

func getPostsID(w http.ResponseWriter, r *http.Request) {
	pidStr := pat.Param(r, "id")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	results := []Post{}

	query := "SELECT p.id AS `id`, p.user_id AS `user_id`, p.body AS `body`, p.mime AS `mime`, p.created_at AS `created_at`, " +
		"u.id AS `user.id`, u.account_name AS `user.account_name`, u.passhash AS `user.passhash`, u.authority AS `user.authority`, u.del_flg AS `user.del_flg`, u.created_at AS `user.created_at` " +
		"FROM `posts` p " +
		"INNER JOIN `users` u ON u.id = p.user_id " +
		"WHERE u.del_flg = 0 AND p.id = ?"
	err = db.Select(&results, query, pid)
	if err != nil {
		log.Print(err)
		return
	}

	if len(results) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	p := results[0]

	var commentCount int
	key := fmt.Sprintf("comment_count:post_id:%d", pid)
	err = mycache.Get(context.Background(), key, &commentCount)
	if err != nil && err != cache.ErrCacheMiss {
		log.Print(err)
		return
	}
	if err == cache.ErrCacheMiss {
		err = db.Get(&commentCount, "SELECT COUNT(*) AS `count` FROM `comments` WHERE `post_id` = ?", p.ID)
		if err != nil {
			log.Print(err)
			return
		}

		err = mycache.Set(&cache.Item{
			Ctx:   context.Background(),
			Key:   key,
			Value: commentCount,
			TTL:   10 * time.Second,
		})
		if err != nil {
			log.Print(err)
			return
		}
	}
	p.CommentCount = commentCount

	var comments []Comment
	key = fmt.Sprintf("comments:post_id:%d", p.ID)
	err = mycache.Get(context.Background(), key, &comments)
	if err != nil && err != cache.ErrCacheMiss {
		log.Print(err)
		return
	}
	if err == cache.ErrCacheMiss {
		query := "SELECT c.id AS `id`, c.post_id AS `post_id`, c.user_id AS `user_id`, c.comment AS `comment`, c.created_at AS `created_at`, " +
			"u.id AS `user.id`, u.account_name AS `user.account_name`, u.passhash AS `user.passhash`, u.authority AS `user.authority`, u.del_flg AS `user.del_flg`, u.created_at AS `user.created_at` " +
			"FROM `comments` c " +
			"INNER JOIN `users` u ON u.id = c.user_id " +
			"WHERE c.post_id = ? ORDER BY c.created_at DESC LIMIT 3"
		err = db.Select(&comments, query, p.ID)
		if err != nil {
			log.Print(err)
			return
		}

		// reverse
		for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
			comments[i], comments[j] = comments[j], comments[i]
		}

		err = mycache.Set(&cache.Item{
			Ctx:   context.Background(),
			Key:   key,
			Value: comments,
			TTL:   10 * time.Second,
		})
		if err != nil {
			log.Print(err)
			return
		}
	}

	p.CSRFToken = getCSRFToken(r)

	me := getSessionUser(r)

	fmap := template.FuncMap{
		"imageURL": imageURL,
	}

	template.Must(template.New("layout.html").Funcs(fmap).ParseFiles(
		getTemplPath("layout.html"),
		getTemplPath("post_id.html"),
		getTemplPath("post.html"),
	)).Execute(w, struct {
		Post Post
		Me   User
	}{p, me})
}

func postIndex(w http.ResponseWriter, r *http.Request) {
	me := getSessionUser(r)
	if !isLogin(me) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if r.FormValue("csrf_token") != getCSRFToken(r) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		session := getSession(r)
		session.Values["notice"] = "画像が必須です"
		session.Save(r, w)

		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	mime, ext := "", ""
	if file != nil {
		// 投稿のContent-Typeからファイルのタイプを決定する
		contentType := header.Header["Content-Type"][0]
		if strings.Contains(contentType, "jpeg") {
			mime = "image/jpeg"
			ext = "jpg"
		} else if strings.Contains(contentType, "png") {
			mime = "image/png"
			ext = "png"
		} else if strings.Contains(contentType, "gif") {
			mime = "image/gif"
			ext = "gif"
		} else {
			session := getSession(r)
			session.Values["notice"] = "投稿できる画像形式はjpgとpngとgifだけです"
			session.Save(r, w)

			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}

	filedata, err := io.ReadAll(file)
	if err != nil {
		log.Print(err)
		return
	}

	if len(filedata) > UploadLimit {
		session := getSession(r)
		session.Values["notice"] = "ファイルサイズが大きすぎます"
		session.Save(r, w)

		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	query := "INSERT INTO `posts` (`user_id`, `mime`, `imgdata`, `body`) VALUES (?,?,?,?)"
	result, err := db.Exec(
		query,
		me.ID,
		mime,
		"",
		r.FormValue("body"),
	)
	if err != nil {
		log.Print(err)
		return
	}

	pid, err := result.LastInsertId()
	if err != nil {
		log.Print(err)
		return
	}

	pidStr := strconv.FormatInt(pid, 10)
	err = os.WriteFile(filepath.Join("..", "public", "image", getFilename(pidStr, ext)), filedata, os.ModePerm)
	if err != nil {
		log.Print(err)
		return
	}

	http.Redirect(w, r, "/posts/"+pidStr, http.StatusFound)
}

func getImage(w http.ResponseWriter, r *http.Request) {
	pidStr := pat.Param(r, "id")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	post := Post{}
	err = db.Get(&post, "SELECT * FROM `posts` WHERE `id` = ?", pid)
	if err != nil {
		log.Print(err)
		return
	}

	ext := pat.Param(r, "ext")

	if ext == "jpg" && post.Mime == "image/jpeg" ||
		ext == "png" && post.Mime == "image/png" ||
		ext == "gif" && post.Mime == "image/gif" {
		w.Header().Set("Content-Type", post.Mime)
		_, err := w.Write(post.Imgdata)
		if err != nil {
			log.Print(err)
			return
		}

		err = os.WriteFile(filepath.Join("..", "public", "image", getFilename(pidStr, ext)), post.Imgdata, os.ModePerm)
		if err != nil {
			log.Print(err)
			return
		}
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func getFilename(id, ext string) string {
	return id + "." + ext
}

func postComment(w http.ResponseWriter, r *http.Request) {
	me := getSessionUser(r)
	if !isLogin(me) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if r.FormValue("csrf_token") != getCSRFToken(r) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	postID, err := strconv.Atoi(r.FormValue("post_id"))
	if err != nil {
		log.Print("post_idは整数のみです")
		return
	}

	query := "INSERT INTO `comments` (`post_id`, `user_id`, `comment`) VALUES (?,?,?)"
	_, err = db.Exec(query, postID, me.ID, r.FormValue("comment"))
	if err != nil {
		log.Print(err)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/posts/%d", postID), http.StatusFound)
}

func getAdminBanned(w http.ResponseWriter, r *http.Request) {
	me := getSessionUser(r)
	if !isLogin(me) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if me.Authority == 0 {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	users := []User{}
	err := db.Select(&users, "SELECT * FROM `users` WHERE `authority` = 0 AND `del_flg` = 0 ORDER BY `created_at` DESC")
	if err != nil {
		log.Print(err)
		return
	}

	template.Must(template.ParseFiles(
		getTemplPath("layout.html"),
		getTemplPath("banned.html")),
	).Execute(w, struct {
		Users     []User
		Me        User
		CSRFToken string
	}{users, me, getCSRFToken(r)})
}

func postAdminBanned(w http.ResponseWriter, r *http.Request) {
	me := getSessionUser(r)
	if !isLogin(me) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if me.Authority == 0 {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if r.FormValue("csrf_token") != getCSRFToken(r) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	query := "UPDATE `users` SET `del_flg` = ? WHERE `id` = ?"

	err := r.ParseForm()
	if err != nil {
		log.Print(err)
		return
	}

	for _, id := range r.Form["uid[]"] {
		db.Exec(query, 1, id)
	}

	http.Redirect(w, r, "/admin/banned", http.StatusFound)
}

type RegexpPattern struct {
	regexp *regexp.Regexp
}

func Regexp(reg *regexp.Regexp) *RegexpPattern {
	return &RegexpPattern{regexp: reg}
}

func (reg *RegexpPattern) Match(r *http.Request) *http.Request {
	ctx := r.Context()
	uPath := pattern.Path(ctx)
	if reg.regexp.MatchString(uPath) {
		values := reg.regexp.FindStringSubmatch(uPath)
		keys := reg.regexp.SubexpNames()

		for i := 1; i < len(keys); i++ {
			ctx = context.WithValue(ctx, pattern.Variable(keys[i]), values[i])
		}

		return r.WithContext(ctx)
	}

	return nil
}

func main() {
	host := os.Getenv("ISUCONP_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("ISUCONP_DB_PORT")
	if port == "" {
		port = "3306"
	}
	_, err := strconv.Atoi(port)
	if err != nil {
		log.Fatalf("Failed to read DB port number from an environment variable ISUCONP_DB_PORT.\nError: %s", err.Error())
	}
	user := os.Getenv("ISUCONP_DB_USER")
	if user == "" {
		user = "root"
	}
	password := os.Getenv("ISUCONP_DB_PASSWORD")
	dbname := os.Getenv("ISUCONP_DB_NAME")
	if dbname == "" {
		dbname = "isuconp"
	}

	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local&interpolateParams=true",
		user,
		password,
		host,
		port,
		dbname,
	)

	db, err = sqlx.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %s.", err.Error())
	}
	defer db.Close()

	ring := redis.NewRing(&redis.RingOptions{
		Addrs: map[string]string{
			"server1": ":6379",
		},
	})
	mycache = cache.New(&cache.Options{
		Redis: ring,
	})

	mux := goji.NewMux()

	mux.HandleFunc(pat.Get("/initialize"), getInitialize)
	mux.HandleFunc(pat.Get("/login"), getLogin)
	mux.HandleFunc(pat.Post("/login"), postLogin)
	mux.HandleFunc(pat.Get("/register"), getRegister)
	mux.HandleFunc(pat.Post("/register"), postRegister)
	mux.HandleFunc(pat.Get("/logout"), getLogout)
	mux.HandleFunc(pat.Get("/"), getIndex)
	mux.HandleFunc(pat.Get("/posts"), getPosts)
	mux.HandleFunc(pat.Get("/posts/:id"), getPostsID)
	mux.HandleFunc(pat.Post("/"), postIndex)
	mux.HandleFunc(pat.Get("/image/:id.:ext"), getImage)
	mux.HandleFunc(pat.Post("/comment"), postComment)
	mux.HandleFunc(pat.Get("/admin/banned"), getAdminBanned)
	mux.HandleFunc(pat.Post("/admin/banned"), postAdminBanned)
	mux.HandleFunc(Regexp(regexp.MustCompile(`^/@(?P<accountName>[a-zA-Z]+)$`)), getAccountName)
	mux.Handle(pat.Get("/*"), http.FileServer(http.Dir("../public")))

	log.Fatal(http.ListenAndServe(":8080", mux))
}