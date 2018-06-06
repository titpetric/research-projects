package main

import (
	"fmt"
	"net/http"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/kardianos/osext"
)

var db *sql.DB

func signHandler(w http.ResponseWriter, r *http.Request) {
	email := r.PostFormValue("email")
	name := r.PostFormValue("name")
	gitlab := r.PostFormValue("gitlab")

	if email == "" || name == "" || gitlab == "" {
		fmt.Fprintf(w, `{"success": false, "message": "One or more field(s) empty."}`)
		return
	}

	statement := `
		INSERT INTO
		signatures (email, name, gitlab)
		VALUES     ($1,    $2,   $3    );
	`
	_, err := db.Exec(statement, email, name, gitlab)
	if err != nil {
		fmt.Fprintf(w, `{"success": false, "message": "You've already signed this document."}`)
		return
	}

	fmt.Fprintf(w, `{"success": true}`)
}

func hasSignedHandler(w http.ResponseWriter, r *http.Request) {
	gitlab := r.PostFormValue("gitlab")

	if gitlab == "" {
		fmt.Fprintf(w, `{"success": false}`)
		return
	}

	statement := `
		SELECT EXISTS (
			SELECT 1
			FROM signatures
			WHERE gitlab = $1
		);
	`
	row := db.QueryRow(statement, gitlab)

	var exists bool
	if err := row.Scan(&exists); err != nil {
		fmt.Fprintf(w, `{"success": false}`)
		return
	}

	fmt.Fprintf(w, `{"success": true, "hasSigned": %v}`, exists)
}

func main() {
	var err error

	path, err := osext.ExecutableFolder()
	if err != nil {
		fmt.Printf("error: cannot find executable path: %v", err)
		return
	}

	db, err = sql.Open("sqlite3", "dco.sqlite3")
	if err != nil {
		fmt.Printf("error: cannot open dco.sqlite3: %v\n", err)
		return
	}

	statement := `
		CREATE TABLE IF NOT EXISTS signatures (
			email TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			gitlab TEXT NOT NULL UNIQUE PRIMARY KEY, -- overengineered af
			signatureDate DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err = db.Exec(statement)
	if err != nil {
		fmt.Printf("error: cannot create table: %v\n", err)
		return
	}

	http.Handle("/", http.FileServer(http.Dir(path)))
	http.HandleFunc("/api/sign", signHandler)
	http.HandleFunc("/api/has-signed", hasSignedHandler)

	fmt.Printf("listening on port 8080\n")
	http.ListenAndServe(":8080", nil)
}