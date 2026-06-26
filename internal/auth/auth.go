// Пакет auth отвечает за генерацию и проверку токенов аутентификации.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("недействительный токен")
	jwtSecret       = []byte("your-secret-key")
	tokenTTL        = 24 * time.Hour
)

type Claims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken создаёт новый токен для пользователя с указанным ID
func GenerateToken(userID int64) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)

	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateToken проверяет токен и возвращает ID пользователя.
// Возвращает ErrInvalidToken если токен не найден.
func ValidateToken(tokenString string) (int64, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return 0, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return 0, ErrInvalidToken
	}

	return claims.UserID, nil
}

func SetSecretKey(secret string) {
	jwtSecret = []byte(secret)
}

func SetTokenTTL(ttl time.Duration) {
	tokenTTL = ttl
}
