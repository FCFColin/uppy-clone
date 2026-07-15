package crypto

// LooksLikeBcryptHash is a heuristic check — it only verifies the string is 60
// characters and starts with a known bcrypt prefix ($2a$, $2b$, or $2y$).
// It does not validate the cost, salt, or base64 encoding.
// Callers that need strict validation should use bcrypt.CompareHashAndPassword.
func LooksLikeBcryptHash(s string) bool {
	return len(s) == 60 && (s[:4] == "$2a$" || s[:4] == "$2b$" || s[:4] == "$2y$")
}
