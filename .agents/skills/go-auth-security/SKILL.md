---
name: go-auth-security
description: >-
  Use this skill when implementing or reviewing authentication and
  authorization in the Go backend. Covers the pluggable auth provider
  architecture, JWT implementation, password hashing, middleware guards,
  RBAC, and session/token lifecycle management.
---

# Go Auth & Security

## When to Activate

- Implementing or modifying auth handlers (login, register, refresh, logout)
- Writing auth middleware or RBAC guards
- Reviewing password hashing, token generation, or session management
- Adding or switching auth providers (JWT → Clerk, etc.)

---

## Pluggable Auth Architecture

### The Interface

```go
// auth/provider.go
type AuthProvider interface {
    Register(ctx context.Context, req RegisterRequest) (*User, error)
    Login(ctx context.Context, req LoginRequest) (*TokenPair, error)
    RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error)
    ValidateToken(ctx context.Context, accessToken string) (*Claims, error)
    RevokeToken(ctx context.Context, refreshToken string) error
}

type Claims struct {
    UserID string
    Email  string
    Role   string
}

type TokenPair struct {
    AccessToken  string
    RefreshToken string
    ExpiresIn    int // seconds
}
```

### Rules

1. **Domain code depends on `AuthProvider` interface, never on JWT/Clerk
   directly.**
2. **The provider is injected at startup** via configuration.
3. **Switching providers** = new implementation of `AuthProvider` + config change.
   Zero domain code changes.
4. **Claims are the universal identity** — handlers and services receive
   `Claims`, not tokens.

---

## JWT Implementation

### Token Lifecycle

| Token        | Storage       | Lifetime   | Purpose                  |
| ------------ | ------------- | ---------- | ------------------------ |
| Access token | HTTP header   | 15 minutes | Authenticate API requests |
| Refresh token| httpOnly cookie | 7 days  | Obtain new access tokens  |

### Signing

```go
func (p *JWTProvider) generateAccessToken(user *User) (string, error) {
    claims := jwt.MapClaims{
        "sub":   user.ID.String(),
        "email": user.Email,
        "role":  user.Role,
        "iat":   time.Now().Unix(),
        "exp":   time.Now().Add(15 * time.Minute).Unix(),
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(p.secret))
}
```

### Rules

1. **HS256 minimum; RS256 for multi-service.** Never `"none"` algorithm.
2. **Short access token TTL** (15 min). Refresh for longer sessions.
3. **Validate `exp`, `iat`, and signature** on every request.
4. **Secret from env var** — never hard-coded. Rotate periodically.
5. **Store refresh tokens in DB** for explicit revocation.

---

## Password Security

```go
import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    return string(bytes), err
}

func CheckPassword(password, hash string) bool {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
```

### Rules

1. **bcrypt with DefaultCost (10)** minimum. Increase to 12 for high-security.
2. **Never store plaintext passwords.** Not even in logs or error messages.
3. **Constant-time comparison** — `bcrypt.CompareHashAndPassword` does this.
4. **Password policy:** min 8 chars, at least one letter and one digit.
5. **Never return "user not found" vs "wrong password"** — always "invalid
   credentials" to prevent user enumeration.

---

## Auth Middleware

```go
func (m *AuthMiddleware) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        token := extractBearerToken(c.Request())
        if token == "" {
            return echo.NewHTTPError(http.StatusUnauthorized, "missing token")
        }

        claims, err := m.provider.ValidateToken(c.Request().Context(), token)
        if err != nil {
            return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
        }

        // Inject claims into context
        ctx := context.WithValue(c.Request().Context(), ClaimsKey, claims)
        c.SetRequest(c.Request().WithContext(ctx))
        return next(c)
    }
}

func (m *AuthMiddleware) RequireRole(roles ...string) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            claims := GetClaims(c.Request().Context())
            if claims == nil {
                return echo.NewHTTPError(http.StatusUnauthorized)
            }
            for _, role := range roles {
                if claims.Role == role {
                    return next(c)
                }
            }
            return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
        }
    }
}
```

---

## RBAC

| Role     | Can do                                              |
| -------- | --------------------------------------------------- |
| ADMIN    | Everything                                          |
| TEACHER  | CRUD exams, manage questions, assign students, view results |
| STUDENT  | View assigned exams, start sessions, submit answers |
| PROCTOR  | Monitor sessions, flag irregularities               |

### Rules

1. **RBAC enforced at middleware level** — not inside service methods.
2. **Resource ownership checks** in service layer (e.g., teacher can only edit
   their own exams).
3. **Never trust client-sent roles.** Role comes from the token claims.

---

## Common Mistakes

| Mistake                                   | Fix                                            |
| ----------------------------------------- | ---------------------------------------------- |
| "User not found" vs "Wrong password"      | Always "Invalid credentials"                   |
| JWT secret in source code                 | Environment variable, never committed          |
| No refresh token revocation               | Store refresh tokens in DB; delete on logout   |
| Role checks only in frontend              | Backend middleware must enforce RBAC            |
| Access token in localStorage              | Use in-memory on client; httpOnly cookie for refresh |
| Not validating token algorithm            | Pin `jwt.SigningMethodHS256` explicitly         |

---

## Production Checklist

- [ ] Auth provider implements `AuthProvider` interface
- [ ] Access token: 15-min TTL, sent via `Authorization: Bearer`
- [ ] Refresh token: 7-day TTL, httpOnly cookie, stored in DB
- [ ] Passwords hashed with bcrypt (cost ≥ 10)
- [ ] Login error: "Invalid credentials" (no user enumeration)
- [ ] Auth middleware extracts claims into context
- [ ] RBAC middleware on every protected route group
- [ ] Resource ownership verified in service layer
- [ ] JWT secret from env var, never committed
- [ ] Logout revokes refresh token in DB
- [ ] Token algorithm explicitly validated (not `"none"`)
