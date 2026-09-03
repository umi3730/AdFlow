package password

import "golang.org/x/crypto/bcrypt"

type Bcrypt struct{}

func (Bcrypt) Compare(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
