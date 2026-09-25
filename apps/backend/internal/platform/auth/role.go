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

func (r Role) String() string {
	return string(r)
}

func (r Role) IsValid() bool {
	switch r {
	case RoleTeacher, RoleStudent, RoleProctor, RoleAdmin:
		return true
	default:
		return false
	}
}
