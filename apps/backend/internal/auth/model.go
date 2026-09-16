package auth

type Role string

const (
	RoleCandidate Role = "candidate"
	RoleEducator  Role = "educator"
	RoleProctor   Role = "proctor"
	RoleAdmin     Role = "admin"
)

type UserClaims struct {
	Subject     string   `json:"sub"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	Email       string   `json:"email,omitempty"`
	FirstName   string   `json:"firstName,omitempty"`
	LastName    string   `json:"lastName,omitempty"`
	SessionID   string   `json:"sessionId,omitempty"`
}

type contextKey string

const (
	UserClaimsContextKey contextKey = "auth_user_claims"
)
