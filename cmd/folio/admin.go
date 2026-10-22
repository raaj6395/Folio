package main

import (
	"crypto/rand"
	"encoding/base32"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/raaj6395/folio/internal/secret"
	"github.com/raaj6395/folio/internal/store"
)

const adminUsage = `usage: folio admin <command>

  adopt --email <email>            give every notebook without an owner to this account
  reset-password --email <email>   set a new random password and print it once
  disable-user --email <email>     block login and all tokens for this account
  require-owners                   (Postgres) make notebook ownership mandatory; fails if any orphans remain
`

func runAdmin(args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, adminUsage)
		os.Exit(2)
	}
	fs := flag.NewFlagSet("admin "+args[0], flag.ExitOnError)
	email := fs.String("email", "", "account email")
	fs.Parse(args[1:])
	st, err := store.Open(dsn())
	if err != nil {
		fatal(err)
	}
	defer st.Close()
	user := func() store.User {
		if *email == "" {
			fatal(fmt.Errorf("--email is required"))
		}
		u, err := st.UserByEmail(*email)
		if err != nil {
			fatal(fmt.Errorf("account %s: %w", *email, err))
		}
		return u
	}
	switch args[0] {
	case "adopt":
		u := user()
		n, err := st.AdoptOrphans(u.ID)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("%d notebook(s) now belong to %s\n", n, u.Email)
	case "reset-password":
		u := user()
		b := make([]byte, 12)
		rand.Read(b)
		pw := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
		if err := st.SetPassword(u.ID, secret.HashPassword(pw)); err != nil {
			fatal(err)
		}
		if err := st.DeleteUserSessions(u.ID); err != nil {
			fatal(err)
		}
		fmt.Printf("new password for %s (shown once): %s\n", u.Email, pw)
	case "disable-user":
		u := user()
		if err := st.DisableUser(u.ID); err != nil {
			fatal(err)
		}
		fmt.Printf("%s is disabled (sessions removed, tokens rejected)\n", u.Email)
	case "require-owners":
		if err := st.RequireOwners(); err != nil {
			fatal(err)
		}
		fmt.Println("notebook ownership is now mandatory")
	default:
		fmt.Fprint(os.Stderr, adminUsage)
		os.Exit(2)
	}
}
