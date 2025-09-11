package auth

import (

	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// BasicUser represents a user for basic authentication
type BasicUser struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

// AuthConfig represents a single authentication configuration
type AuthConfig struct {
	Group      string      `yaml:"group"`
	Type       string      `yaml:"type"`                 // "jwt" or "basic"
	JwtSecret  string      `yaml:"jwt_secret,omitempty"` // used only if type = jwt
	VerifyExpr string      `yaml:"verify,omitempty"`     // expression for predicate evaluation
	Users      []BasicUser `yaml:"users,omitempty"`      // used only if type = basic
}

// ExtractJWTClaims parses JWT from request and returns claims if valid
func ExtractJWTClaims(r *http.Request, secret string) (map[string]interface{}, bool) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, false
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, false
	}
	tokenStr := parts[1]

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, false
	}

	claimsMap, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, false
	}

	// Convert jwt.MapClaims (map[string]interface{}) to plain map[string]interface{}
	claims := make(map[string]interface{})
	for k, v := range claimsMap {
		claims[k] = v
	}

	return claims, true
}

// ComparePassword verifies the plaintext password against the stored bcrypt hash
func ComparePassword(hashedPassword, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) == nil
}

// EvalPredicate evaluates a Vulcand-style predicate expression against JWT claims
func EvalPredicate(expr string, claims map[string]interface{}) (bool, error) {
	// Simple implementation without using predicate.Compile
	// Supports `have("value")` or `is("key","value")`
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	// Replace 'and' / 'or' with Go operators
	expr = strings.ReplaceAll(expr, " and ", " && ")
	expr = strings.ReplaceAll(expr, " or ", " || ")

	// Split OR/AND for simple evaluation
	clauses := strings.Split(expr, "||")
	for _, clause := range clauses {
		ands := strings.Split(clause, "&&")
		allTrue := true
		for _, a := range ands {
			a = strings.TrimSpace(a)
			ok := false

			if strings.HasPrefix(a, "have(") {
				// e.g., have("admin")
				val := strings.TrimSuffix(strings.TrimPrefix(a, "have("), ")")
				val = strings.Trim(val, `"`)
				ok = false
				for _, v := range claims {
					if s, ok2 := v.(string); ok2 && strings.Contains(s, val) {
						ok = true
						break
					}
				}
			} else if strings.HasPrefix(a, "is(") {
				// e.g., is("group","devops")
				args := strings.TrimSuffix(strings.TrimPrefix(a, "is("), ")")
				parts := strings.Split(args, ",")
				if len(parts) == 2 {
					key := strings.Trim(parts[0], `"`)
					expected := strings.Trim(parts[1], `"`)
					if val, exists := claims[key]; exists {
						if valStr, ok2 := val.(string); ok2 && valStr == expected {
							ok = true
						}
					}
				}
			}

			if !ok {
				allTrue = false
				break
			}
		}

		if allTrue {
			return true, nil
		}
	}

	return false, nil
}

// Helper: format error or default unauthorized message
func AuthErrOrUnauthorized(err error) string {
	if err != nil {
		return err.Error()
	}
	return "unauthorized"
}
