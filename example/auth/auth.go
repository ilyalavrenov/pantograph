package auth

import "errors"

type Session struct {
	User  string
	Token string
}

type Store interface {
	Lookup(token string) (Session, error)
}

type Auth struct {
	store Store
}

func New(store Store) *Auth { return &Auth{store: store} }

//pantograph:auth kind=entry
func (a *Auth) Authenticate(token string) (Session, error) {
	if !checkToken(token) {
		return Session{}, errors.New("unauthorized")
	}

	return a.loadSession(token)
}

//pantograph:auth kind=decision
func checkToken(token string) bool {
	return len(token) >= minTokenLen
}

const minTokenLen = 8

//pantograph:auth kind=store
func (a *Auth) loadSession(token string) (Session, error) {
	return a.store.Lookup(token)
}
