package tokens

import (
	"errors"
	"fmt"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/utils/validate"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

type AuthTokens struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	AccessExpiresIn  time.Time `json:"access_expires_in"`
	RefreshExpiresIn time.Time `json:"refresh_expires_in"`
}

type CustomClaims struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	ID    uint   `json:"id"`
}

func CreateAuthTokens(user *CustomClaims, authCfg *cfg.AuthTokenConfig) (*AuthTokens, error) {
	accessToken, accessExpirationTime, err := createToken(user.ID, user.Email, user.Role, "access", authCfg)
	if err != nil {
		return nil, fmt.Errorf("cannot create access token: %w", err)
	}

	refreshToken, refreshExpirationTime, err := createToken(user.ID, user.Email, user.Role, "refresh", authCfg)
	if err != nil {
		return nil, fmt.Errorf("cannot create refresh token: %w", err)
	}

	res := &AuthTokens{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        "Bearer",
		AccessExpiresIn:  accessExpirationTime,
		RefreshExpiresIn: refreshExpirationTime,
	}

	return res, nil
}

func CheckAccessToken(tokenString string, authCfg *cfg.AuthTokenConfig) (*CustomClaims, error) {
	user, err := checkToken(tokenString, &authCfg.Secret, authCfg.Issuer, "access")
	if err != nil {
		return nil, fmt.Errorf("cannot verify token: %w", err)
	}

	return user, nil
}

func CheckRefreshToken(tokenString string, authCfg *cfg.AuthTokenConfig) (*CustomClaims, error) {
	user, err := checkToken(tokenString, &authCfg.Secret, authCfg.Issuer, "refresh")
	if err != nil {
		return nil, fmt.Errorf("cannot verify token: %w", err)
	}

	return user, nil
}

type claims struct {
	TokenType string
	UserEmail string
	UserRole  string
	UserID    uint
	jwt.RegisteredClaims
}

func checkToken(tokenString string, jwtKey *cfg.Bytes, issuer string, tokenType string) (*CustomClaims, error) {
	claims := &claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (interface{}, error) {
		return []byte(*jwtKey), nil
	})

	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}

	const timeSkew = validate.ServerTimeSkew
	now := time.Now()
	if !claims.VerifyExpiresAt(now.Add(-timeSkew), true) {
		return nil, errors.New("token has expired")
	}

	if !claims.VerifyIssuedAt(now.Add(timeSkew), true) {
		return nil, errors.New("token was issued in the future")
	}

	if !claims.VerifyIssuer(issuer, true) {
		return nil, errors.New("invalid token issuer")
	}

	if claims.TokenType != tokenType {
		return nil, errors.New("invalid token type")
	}

	jwtUser := &CustomClaims{
		Email: claims.UserEmail,
		Role:  claims.UserRole,
		ID:    claims.UserID,
	}

	return jwtUser, nil
}

func createToken(id uint, email, role, tokenType string, jwtConfig *cfg.AuthTokenConfig) (string, time.Time, error) {
	now := time.Now()
	var expirationTime time.Time
	if tokenType == "refresh" {
		expirationTime = now.Add(jwtConfig.RefreshExpirationTime)
	} else {
		expirationTime = now.Add(jwtConfig.AccessExpirationTime)
	}

	claims := &claims{
		TokenType: tokenType,
		UserEmail: email,
		UserRole:  role,
		UserID:    id,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    jwtConfig.Issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(jwtConfig.Secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("cannot sign token: %w", err)
	}

	return tokenString, expirationTime, nil
}
