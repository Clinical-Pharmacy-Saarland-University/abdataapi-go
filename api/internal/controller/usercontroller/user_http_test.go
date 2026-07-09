package usercontroller_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/usercontroller"
	"observeddb-go-api/internal/utils/hash"
	"observeddb-go-api/internal/utils/tokens"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	mysqldriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testSecret is an arbitrary JWT signing key used to mint and verify tokens
// entirely in-process; no configuration file or network access is involved.
const testSecret = "test-secret-key-for-unit-tests"

// newUserController builds a UserController backed by a GORM DB whose underlying
// *sql.DB is a sqlmock connection. The mysql dialector is attached to the
// existing sqlmock connection with SkipInitializeWithVersion so gorm.Open does
// not probe the server. The Mailer is nil on purpose: only handlers/paths that
// return before sending email are exercised, so the mailer is never touched.
func newUserController(t *testing.T) (*usercontroller.UserController, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	gdb, err := gorm.Open(mysqldriver.New(mysqldriver.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		_ = db.Close()
		t.Fatalf("failed to open gorm with sqlmock: %v", err)
	}

	uc := &usercontroller.UserController{
		DB: gdb,
		AuthCfg: cfg.AuthTokenConfig{
			Secret:                cfg.Bytes(testSecret),
			AccessExpirationTime:  15 * time.Minute,
			RefreshExpirationTime: 24 * time.Hour,
			Issuer:                "observeddb-test",
		},
		ResetCfg: cfg.ResetTokenConfig{
			ExpirationTime: time.Hour,
			RetryInterval:  time.Minute,
		},
		// Mailer intentionally left nil; no tested path sends email.
		Mailer: nil,
	}

	cleanup := func() { _ = db.Close() }

	return uc, mock, cleanup
}

// userColumns lists the columns SELECT * returns for the users table, matching
// the model.User struct fields (gorm.Model + explicit fields).
var userColumns = []string{
	"id", "created_at", "updated_at", "deleted_at",
	"email", "last_name", "first_name", "org",
	"role", "status", "last_login", "pwd_hash",
}

// userRow builds a single sqlmock row for the users table with sensible zero
// values for the audit columns and the given business fields.
func userRow(id uint, email, role, status string, pwdHash *string, lastLogin *time.Time) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(userColumns).AddRow(
		id, now, now, nil,
		email, "Doe", "Joe", "ACME",
		role, status, lastLogin, pwdHash,
	)
}

// mustHash returns a valid argon2id hash of the given password, mirroring how
// the application stores password hashes. hash.Check succeeds only for the
// exact same input, so this lets tests exercise the real credential check.
func mustHash(t *testing.T, pwd string) *string {
	t.Helper()
	h, err := hash.Create(pwd)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	return &h
}

// doJSON sends a JSON body to a single handler and returns the recorder. When
// userID is non-zero it is injected into the gin context under "user_id",
// mimicking the auth middleware for the authenticated handlers.
func doJSON(handler gin.HandlerFunc, method, path string, body any, userID uint) *httptest.ResponseRecorder {
	r := gin.New()
	if userID != 0 {
		r.Use(func(c *gin.Context) {
			c.Set("user_id", userID)
		})
	}
	r.Handle(method, path, handler)

	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}

	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// failEnvelope is the {"status":"fail","data":{"error":...}} shape produced by
// handle.newJSendFailure for 4xx client errors below 500.
type failEnvelope struct {
	Status string `json:"status"`
	Data   struct {
		Error string `json:"error"`
	} `json:"data"`
}

// successEnvelope is the {"status":"success","data":...} wrapper; Data is left
// as raw JSON so each test can decode the payload it expects.
type successEnvelope struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

// --- Login -----------------------------------------------------------------

// TestLogin_Success verifies that a matching argon2 password on an active user
// yields 200 with a full AuthTokens payload and the user's role/last_login. The
// handler also fires UpdateLastLogin, so a Begin/Exec/Commit is expected after
// the lookup.
func TestLogin_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	const pwd = "correct horse battery"
	pwdHash := mustHash(t, pwd)

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs("joe@me.com", 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", pwdHash, nil))

	// UpdateLastLogin runs in its own transaction and its error is ignored by
	// the handler, but the statement is still issued and must be satisfied.
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	w := doJSON(uc.Login, http.MethodPost, "/user/login",
		map[string]string{"login": "joe@me.com", "password": pwd}, 0)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var env successEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Status != "success" {
		t.Errorf("status = %q, want success", env.Status)
	}

	var payload struct {
		tokens.AuthTokens
		Role string `json:"role"`
	}
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatalf("decode data %q: %v", string(env.Data), err)
	}
	if payload.AccessToken == "" || payload.RefreshToken == "" {
		t.Errorf("expected non-empty tokens, got %+v", payload)
	}
	if payload.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", payload.TokenType)
	}
	if payload.Role != "user" {
		t.Errorf("role = %q, want user", payload.Role)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestLogin_WrongPassword verifies a wrong password is rejected with 401 and
// the generic "Invalid credentials" message, without any UpdateLastLogin write.
func TestLogin_WrongPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	pwdHash := mustHash(t, "the-real-password")

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs("joe@me.com", 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", pwdHash, nil))

	w := doJSON(uc.Login, http.MethodPost, "/user/login",
		map[string]string{"login": "joe@me.com", "password": "wrong-password"}, 0)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusUnauthorized, w.Code, w.Body.String())
	}

	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Status != "fail" || env.Data.Error != "Invalid credentials" {
		t.Errorf("got %+v, want fail/Invalid credentials", env)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB write on wrong password: %v", err)
	}
}

// TestLogin_UnknownUser verifies that a lookup miss (gorm ErrRecordNotFound)
// maps to 401 Invalid credentials, never revealing whether the email exists.
func TestLogin_UnknownUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs("ghost@me.com", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	w := doJSON(uc.Login, http.MethodPost, "/user/login",
		map[string]string{"login": "ghost@me.com", "password": "whatever"}, 0)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusUnauthorized, w.Code, w.Body.String())
	}

	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Data.Error != "Invalid credentials" {
		t.Errorf("error = %q, want Invalid credentials", env.Data.Error)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestLogin_NoPasswordSet verifies a user whose PwdHash is NULL (never set a
// password) is treated as invalid credentials rather than crashing.
func TestLogin_NoPasswordSet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs("joe@me.com", 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", nil, nil))

	w := doJSON(uc.Login, http.MethodPost, "/user/login",
		map[string]string{"login": "joe@me.com", "password": "anything"}, 0)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusUnauthorized, w.Code, w.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestLogin_InactiveUser verifies that valid credentials on a non-active
// account are rejected with 403 after the password check passes.
func TestLogin_InactiveUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	const pwd = "valid-password"
	pwdHash := mustHash(t, pwd)

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs("joe@me.com", 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "inactive", pwdHash, nil))

	w := doJSON(uc.Login, http.MethodPost, "/user/login",
		map[string]string{"login": "joe@me.com", "password": pwd}, 0)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusForbidden, w.Code, w.Body.String())
	}

	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Data.Error != "User account is not active" {
		t.Errorf("error = %q, want User account is not active", env.Data.Error)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestLogin_RoleDowngrade verifies a "user" logging in cannot escalate to
// "admin": switchRole rejects the request via CanSwitchToRole and the handler
// returns 403 before minting a token.
func TestLogin_RoleEscalationForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	const pwd = "valid-password"
	pwdHash := mustHash(t, pwd)
	admin := "admin"

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs("joe@me.com", 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", pwdHash, nil))

	w := doJSON(uc.Login, http.MethodPost, "/user/login",
		map[string]any{"login": "joe@me.com", "password": pwd, "role": admin}, 0)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusForbidden, w.Code, w.Body.String())
	}

	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Data.Error != "Unauthorized role access" {
		t.Errorf("error = %q, want Unauthorized role access", env.Data.Error)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestLogin_MissingFields verifies that omitting required body fields fails
// binding with 422 before any DB query runs.
func TestLogin_MissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body map[string]string
	}{
		{name: "missing password", body: map[string]string{"login": "joe@me.com"}},
		{name: "missing login", body: map[string]string{"password": "secret"}},
		{name: "empty body", body: map[string]string{}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			uc, mock, cleanup := newUserController(t)
			defer cleanup()

			// No ExpectQuery: binding must fail before the DB is touched.
			w := doJSON(uc.Login, http.MethodPost, "/user/login", tt.body, 0)

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected %d, got %d (body: %s)",
					http.StatusUnprocessableEntity, w.Code, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unexpected DB interaction on invalid input: %v", err)
			}
		})
	}
}

// TestLogin_InvalidRoleValue verifies the oneof binding tag rejects an unknown
// role value with 422 during binding.
func TestLogin_InvalidRoleValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	w := doJSON(uc.Login, http.MethodPost, "/user/login",
		map[string]any{"login": "joe@me.com", "password": "secret", "role": "superuser"}, 0)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected %d, got %d (body: %s)",
			http.StatusUnprocessableEntity, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction on invalid role: %v", err)
	}
}

// --- RefreshToken ----------------------------------------------------------

// makeRefreshToken mints a valid refresh token for the given identity using the
// same AuthCfg the controller was built with, so CheckRefreshToken accepts it.
func makeRefreshToken(t *testing.T, uc *usercontroller.UserController, id uint, email, role string) string {
	t.Helper()
	pair, err := tokens.CreateAuthTokens(&tokens.CustomClaims{ID: id, Email: email, Role: role}, &uc.AuthCfg)
	if err != nil {
		t.Fatalf("failed to create tokens: %v", err)
	}
	return pair.RefreshToken
}

// TestRefreshToken_Success verifies a valid refresh token for an active user
// yields fresh tokens (200). The lookup is by ID and UpdateLastLogin fires.
func TestRefreshToken_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	refresh := makeRefreshToken(t, uc, 42, "joe@me.com", "user")

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", mustHash(t, "pw"), nil))

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	w := doJSON(uc.RefreshToken, http.MethodPost, "/user/refresh-token",
		map[string]string{"refresh_token": refresh}, 0)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var env successEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	var payload tokens.AuthTokens
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatalf("decode data %q: %v", string(env.Data), err)
	}
	if payload.AccessToken == "" || payload.RefreshToken == "" {
		t.Errorf("expected fresh tokens, got %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestRefreshToken_Invalid verifies a garbage token is rejected with 401 before
// any DB lookup.
func TestRefreshToken_Invalid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	w := doJSON(uc.RefreshToken, http.MethodPost, "/user/refresh-token",
		map[string]string{"refresh_token": "not-a-jwt"}, 0)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusUnauthorized, w.Code, w.Body.String())
	}

	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Data.Error != "Invalid refresh token" {
		t.Errorf("error = %q, want Invalid refresh token", env.Data.Error)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction on invalid token: %v", err)
	}
}

// TestRefreshToken_AccessTokenRejected verifies that presenting an *access*
// token where a refresh token is expected is rejected: CheckRefreshToken
// enforces the token type, so the result is 401 with no DB lookup.
func TestRefreshToken_AccessTokenRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	pair, err := tokens.CreateAuthTokens(&tokens.CustomClaims{ID: 42, Email: "joe@me.com", Role: "user"}, &uc.AuthCfg)
	if err != nil {
		t.Fatalf("failed to create tokens: %v", err)
	}

	w := doJSON(uc.RefreshToken, http.MethodPost, "/user/refresh-token",
		map[string]string{"refresh_token": pair.AccessToken}, 0)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusUnauthorized, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction: %v", err)
	}
}

// TestRefreshToken_UserNotFound verifies a valid token whose user no longer
// exists yields 403 "User not found".
func TestRefreshToken_UserNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	refresh := makeRefreshToken(t, uc, 99, "gone@me.com", "user")

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(99, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	w := doJSON(uc.RefreshToken, http.MethodPost, "/user/refresh-token",
		map[string]string{"refresh_token": refresh}, 0)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusForbidden, w.Code, w.Body.String())
	}

	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Data.Error != "User not found" {
		t.Errorf("error = %q, want User not found", env.Data.Error)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestRefreshToken_InactiveUser verifies a valid token for a now-inactive user
// is rejected with 403.
func TestRefreshToken_InactiveUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	refresh := makeRefreshToken(t, uc, 42, "joe@me.com", "user")

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "inactive", mustHash(t, "pw"), nil))

	w := doJSON(uc.RefreshToken, http.MethodPost, "/user/refresh-token",
		map[string]string{"refresh_token": refresh}, 0)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusForbidden, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestRefreshToken_MissingField verifies an empty body fails binding with 422.
func TestRefreshToken_MissingField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	w := doJSON(uc.RefreshToken, http.MethodPost, "/user/refresh-token",
		map[string]string{}, 0)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected %d, got %d (body: %s)",
			http.StatusUnprocessableEntity, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction on missing field: %v", err)
	}
}

// --- ChangePwd -------------------------------------------------------------

// TestChangePwd_Success verifies the authenticated user can change their
// password: the old password matches, the new one passes validation, and the
// user is saved. The handler looks up by ID then Saves in a transaction.
func TestChangePwd_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	const oldPwd = "old-password-1"
	oldHash := mustHash(t, oldPwd)

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", oldHash, nil))

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	w := doJSON(uc.ChangePwd, http.MethodPatch, "/user/password",
		map[string]string{"old_password": oldPwd, "new_password": "brand-new-password"}, 42)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestChangePwd_WrongOldPassword verifies a wrong current password yields 400
// and does not write to the DB.
func TestChangePwd_WrongOldPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	oldHash := mustHash(t, "the-actual-old-password")

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", oldHash, nil))

	w := doJSON(uc.ChangePwd, http.MethodPatch, "/user/password",
		map[string]string{"old_password": "wrong-old", "new_password": "brand-new-password"}, 42)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}

	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Data.Error != "Invalid old password" {
		t.Errorf("error = %q, want Invalid old password", env.Data.Error)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB write on wrong old password: %v", err)
	}
}

// TestChangePwd_InvalidNewPassword verifies that a new password failing the
// length policy is rejected with 400 after the old password check passes, and
// before any save.
func TestChangePwd_InvalidNewPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	const oldPwd = "old-password-1"
	oldHash := mustHash(t, oldPwd)

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", oldHash, nil))

	w := doJSON(uc.ChangePwd, http.MethodPatch, "/user/password",
		map[string]string{"old_password": oldPwd, "new_password": "short"}, 42)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}

	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Data.Error != "Invalid new password" {
		t.Errorf("error = %q, want Invalid new password", env.Data.Error)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB write on invalid new password: %v", err)
	}
}

// TestChangePwd_MissingFields verifies missing body fields fail binding (422)
// before any DB lookup.
func TestChangePwd_MissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	w := doJSON(uc.ChangePwd, http.MethodPatch, "/user/password",
		map[string]string{"old_password": "only-old"}, 42)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected %d, got %d (body: %s)",
			http.StatusUnprocessableEntity, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction on missing fields: %v", err)
	}
}

// --- GetProfile ------------------------------------------------------------

// TestGetProfile_Success verifies the profile projection is returned for the
// authenticated user (200) with the expected fields.
func TestGetProfile_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnRows(userRow(42, "joe@me.com", "approver", "active", mustHash(t, "pw"), nil))

	w := doJSON(uc.GetProfile, http.MethodGet, "/user/profile", nil, 42)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var env successEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	var profile struct {
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Org       string `json:"organization"`
		Role      string `json:"role"`
	}
	if err := json.Unmarshal(env.Data, &profile); err != nil {
		t.Fatalf("decode data %q: %v", string(env.Data), err)
	}
	if profile.Email != "joe@me.com" {
		t.Errorf("email = %q, want joe@me.com", profile.Email)
	}
	if profile.Role != "approver" {
		t.Errorf("role = %q, want approver", profile.Role)
	}
	if profile.FirstName != "Joe" || profile.LastName != "Doe" || profile.Org != "ACME" {
		t.Errorf("unexpected profile fields: %+v", profile)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGetProfile_UserLookupError verifies a DB lookup failure surfaces as a 500
// server error.
func TestGetProfile_UserLookupError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	w := doJSON(uc.GetProfile, http.MethodGet, "/user/profile", nil, 42)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected %d, got %d (body: %s)",
			http.StatusInternalServerError, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// --- UpdateProfile ---------------------------------------------------------

// TestUpdateProfile_Success verifies a valid field update looks up the user and
// saves the changes (200).
func TestUpdateProfile_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", mustHash(t, "pw"), nil))

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	w := doJSON(uc.UpdateProfile, http.MethodPatch, "/user/profile",
		map[string]string{"first_name": "Jane"}, 42)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateProfile_NoData verifies that a body with no updatable field is
// rejected with 400 before any DB access.
func TestUpdateProfile_NoData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	w := doJSON(uc.UpdateProfile, http.MethodPatch, "/user/profile",
		map[string]string{}, 42)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}

	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Data.Error != "No data to update provided" {
		t.Errorf("error = %q, want No data to update provided", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction with no data: %v", err)
	}
}

// TestUpdateProfile_InvalidName verifies an invalid name value (contains a
// digit) is rejected with 400 after lookup, before save.
func TestUpdateProfile_InvalidName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", mustHash(t, "pw"), nil))

	w := doJSON(uc.UpdateProfile, http.MethodPatch, "/user/profile",
		map[string]string{"first_name": "Jane123"}, 42)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestUpdateProfile_TooShortField verifies the min=2 binding tag rejects a
// one-character field with 422 during binding.
func TestUpdateProfile_TooShortField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	w := doJSON(uc.UpdateProfile, http.MethodPatch, "/user/profile",
		map[string]string{"first_name": "J"}, 42)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected %d, got %d (body: %s)",
			http.StatusUnprocessableEntity, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction on binding failure: %v", err)
	}
}

// --- DeleteAccount ---------------------------------------------------------

// TestDeleteAccount_Success verifies a non-admin user is soft-deleted: lookup
// by ID, then a transaction that issues the soft-delete UPDATE plus the
// AfterDelete cleanup DELETEs on associated tables.
func TestDeleteAccount_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnRows(userRow(42, "joe@me.com", "user", "active", mustHash(t, "pw"), nil))

	mock.ExpectBegin()
	// Soft delete of the user record (UPDATE ... SET deleted_at).
	mock.ExpectExec("UPDATE `users`").WillReturnResult(sqlmock.NewResult(0, 1))
	// AfterDelete hook cleans up associated pwd-reset and email-change rows.
	mock.ExpectExec("DELETE FROM `user_pwd_resets`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `user_email_changes`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	w := doJSON(uc.DeleteAccount, http.MethodDelete, "/user", nil, 42)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var env successEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Status != "success" {
		t.Errorf("status = %q, want success", env.Status)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestDeleteAccount_LastAdmin verifies deleting the only active admin is blocked
// with 400: the admin count query returns 1, the transaction rolls back, and no
// delete is performed.
func TestDeleteAccount_LastAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(7, 1).
		WillReturnRows(userRow(7, "boss@me.com", "admin", "active", mustHash(t, "pw"), nil))

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	w := doJSON(uc.DeleteAccount, http.MethodDelete, "/user", nil, 7)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}

	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	if env.Data.Error != "Cannot delete the only admin account" {
		t.Errorf("error = %q, want Cannot delete the only admin account", env.Data.Error)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestDeleteAccount_AdminWithPeers verifies an admin can be deleted when other
// active admins remain: the count query returns >1, then the soft-delete
// proceeds.
func TestDeleteAccount_AdminWithPeers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(7, 1).
		WillReturnRows(userRow(7, "boss@me.com", "admin", "active", mustHash(t, "pw"), nil))

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectExec("UPDATE `users`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `user_pwd_resets`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `user_email_changes`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	w := doJSON(uc.DeleteAccount, http.MethodDelete, "/user", nil, 7)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestDeleteAccount_UserLookupError verifies a lookup failure returns 500 before
// any transaction begins.
func TestDeleteAccount_UserLookupError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc, mock, cleanup := newUserController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WithArgs(42, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	w := doJSON(uc.DeleteAccount, http.MethodDelete, "/user", nil, 42)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected %d, got %d (body: %s)",
			http.StatusInternalServerError, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
