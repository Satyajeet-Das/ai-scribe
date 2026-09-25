package auth

type Role string

const (
	RoleTeacher   Role = "TEACHER"
	RoleStudent   Role = "STUDENT"
	RoleEducator  Role = "TEACHER"
	RoleCandidate Role = "STUDENT"
	RoleProctor   Role = "PROCTOR"
	RoleAdmin     Role = "ADMIN"
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
