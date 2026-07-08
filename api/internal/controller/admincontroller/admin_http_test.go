package admincontroller_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/admincontroller"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// userColumns lists the users table columns in the order GORM maps the
// model.User struct, so sqlmock rows scan cleanly into a SELECT *.
var userColumns = []string{
	"id", "created_at", "updated_at", "deleted_at",
	"email", "last_name", "first_name", "org", "role", "status",
	"last_login", "pwd_hash",
}

// newAdminController wires an AdminController to a GORM handle backed by a
// database/sql mock. SkipInitializeWithVersion keeps GORM from probing the
// server version at open time so no query escapes to a real database. The
// Mailer is left nil: every handler exercised here either fails before the
// mailer is reached or never uses it (see the CreateUser note in the report).
func newAdminController(t *testing.T) (*admincontroller.AdminController, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm over sqlmock: %v", err)
	}

	ac := &admincontroller.AdminController{
		DB:       gormDB,
		ResetCfg: cfg.ResetTokenConfig{ExpirationTime: time.Hour},
	}

	cleanup := func() { _ = sqlDB.Close() }
	return ac, mock, cleanup
}

// ctxValues optionally injects the gin context keys the middleware would
// normally set (used by DeleteUserByEmail and ChangeUserProfile).
type ctxValues struct {
	userEmail string
	userID    uint
	setEmail  bool
	setID     bool
}

// serve dispatches a single request against a fresh gin engine wired to one
// handler. The route uses an :email path param so the handlers that read
// c.Param("email") resolve correctly.
func serve(
	method, path string,
	handler gin.HandlerFunc,
	target, body string,
	vals ctxValues,
) *httptest.ResponseRecorder {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if vals.setEmail {
			c.Set("user_email", vals.userEmail)
		}
		if vals.setID {
			c.Set("user_id", vals.userID)
		}
	})
	r.Handle(method, path, handler)

	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// successEnvelope decodes the {"status":"success","data":{"message":...}}
// shape produced by handle.Success for the write handlers.
type successEnvelope struct {
	Status string `json:"status"`
	Data   struct {
		Message string `json:"message"`
	} `json:"data"`
}

// failEnvelope decodes the {"status":"fail","data":{"error":...}} shape
// produced for 400/403/404 responses.
type failEnvelope struct {
	Status string `json:"status"`
	Data   struct {
		Error string `json:"error"`
	} `json:"data"`
}

func decodeSuccess(t *testing.T, w *httptest.ResponseRecorder) successEnvelope {
	t.Helper()
	var env successEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode success body %q: %v", w.Body.String(), err)
	}
	return env
}

func decodeFail(t *testing.T, w *httptest.ResponseRecorder) failEnvelope {
	t.Helper()
	var env failEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode fail body %q: %v", w.Body.String(), err)
	}
	return env
}

// ---------------------------------------------------------------------------
// CreateServiceUser
// ---------------------------------------------------------------------------

// TestCreateServiceUser_Success exercises the full happy path: after JSON bind
// and field validation, IsEmailAvailable issues two SELECTs (users, then
// user_email_changes), both empty, then the user row is inserted. All of this
// runs inside the explicit ac.DB.Transaction wrapper, so BEGIN/COMMIT bracket
// the statements. CreateServiceUser never touches the mailer.
func TestCreateServiceUser_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT 1 FROM `users`").
		WithArgs("joe@gmail.com", 1).
		WillReturnRows(sqlmock.NewRows([]string{"1"}))
	mock.ExpectQuery("SELECT 1 FROM `user_email_changes`").
		WithArgs("joe@gmail.com", 1).
		WillReturnRows(sqlmock.NewRows([]string{"1"}))
	mock.ExpectExec("INSERT INTO `users`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := `{"email":"joe@gmail.com","first_name":"Joe","last_name":"Doe",` +
		`"organization":"ACME","role":"user","password":"password123"}`
	w := serve(http.MethodPost, "/admin/users/service", ac.CreateServiceUser,
		"/admin/users/service", body, ctxValues{})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	env := decodeSuccess(t, w)
	if env.Status != "success" {
		t.Errorf("expected status success, got %q", env.Status)
	}
	if env.Data.Message != "Service user created" {
		t.Errorf("expected message %q, got %q", "Service user created", env.Data.Message)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestCreateServiceUser_EmailInUse verifies that when the users lookup returns
// a row, IsEmailAvailable reports the email taken; the handler returns 400 and
// rolls the transaction back without ever attempting an INSERT.
func TestCreateServiceUser_EmailInUse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT 1 FROM `users`").
		WithArgs("joe@gmail.com", 1).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(true))
	mock.ExpectRollback()

	body := `{"email":"joe@gmail.com","first_name":"Joe","last_name":"Doe",` +
		`"organization":"ACME","role":"user","password":"password123"}`
	w := serve(http.MethodPost, "/admin/users/service", ac.CreateServiceUser,
		"/admin/users/service", body, ctxValues{})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if env := decodeFail(t, w); env.Data.Error != "Email already in use" {
		t.Errorf("expected error %q, got %q", "Email already in use", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestCreateServiceUser_BindValidation covers the two rejection layers that run
// before any DB access: gin binding (422 for missing/invalid tag) and the
// explicit validate.* calls (400). No query is registered so any DB touch fails
// ExpectationsWereMet.
func TestCreateServiceUser_BindValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "missing required field (role) -> 422 binding",
			body:     `{"email":"joe@gmail.com","first_name":"Joe","last_name":"Doe","organization":"ACME","password":"password123"}`,
			wantCode: http.StatusUnprocessableEntity,
		},
		{
			name:     "bad role enum -> 422 binding",
			body:     `{"email":"joe@gmail.com","first_name":"Joe","last_name":"Doe","organization":"ACME","role":"superuser","password":"password123"}`,
			wantCode: http.StatusUnprocessableEntity,
		},
		{
			name:     "malformed json -> 400",
			body:     `{"email":`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid password (too short) -> 400 validate",
			body:     `{"email":"joe@gmail.com","first_name":"Joe","last_name":"Doe","organization":"ACME","role":"user","password":"short"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid first name (digits) -> 400 validate",
			body:     `{"email":"joe@gmail.com","first_name":"Joe123","last_name":"Doe","organization":"ACME","role":"user","password":"password123"}`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ac, mock, cleanup := newAdminController(t)
			defer cleanup()

			w := serve(http.MethodPost, "/admin/users/service", ac.CreateServiceUser,
				"/admin/users/service", tt.body, ctxValues{})

			if w.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d (body: %s)", tt.wantCode, w.Code, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unexpected DB interaction: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CreateUser
// ---------------------------------------------------------------------------

// TestCreateUser_EmailInUse drives CreateUser as far as it can go without a
// real mailer: binding and validation pass, a reset-token pair is generated,
// and IsEmailAvailable finds the address already taken so the handler returns
// 400 and rolls back. The success branch is intentionally not exercised here
// because it calls Mailer.SendNewAccoundEmail, which requires SendGrid.
func TestCreateUser_EmailInUse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT 1 FROM `users`").
		WithArgs("joe@gmail.com", 1).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(true))
	mock.ExpectRollback()

	body := `{"email":"joe@gmail.com","first_name":"Joe","last_name":"Doe",` +
		`"organization":"ACME","role":"user"}`
	w := serve(http.MethodPost, "/admin/users", ac.CreateUser,
		"/admin/users", body, ctxValues{})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if env := decodeFail(t, w); env.Data.Error != "Email already in use" {
		t.Errorf("expected error %q, got %q", "Email already in use", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestCreateUser_BindValidation covers CreateUser's pre-DB rejection layers.
// CreateUser has no password field, so its validation set is email, first
// name, last name and organization.
func TestCreateUser_BindValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "missing required field (email) -> 422 binding",
			body:     `{"first_name":"Joe","last_name":"Doe","organization":"ACME","role":"user"}`,
			wantCode: http.StatusUnprocessableEntity,
		},
		{
			name:     "invalid organization (bad char) -> 400 validate",
			body:     `{"email":"joe@gmail.com","first_name":"Joe","last_name":"Doe","organization":"AC/ME","role":"user"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid last name (leading space passes binding, fails validate) -> 400",
			body:     `{"email":"joe@gmail.com","first_name":"Joe","last_name":"Doe1","organization":"ACME","role":"user"}`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ac, mock, cleanup := newAdminController(t)
			defer cleanup()

			w := serve(http.MethodPost, "/admin/users", ac.CreateUser,
				"/admin/users", tt.body, ctxValues{})

			if w.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d (body: %s)", tt.wantCode, w.Code, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unexpected DB interaction: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetUsers
// ---------------------------------------------------------------------------

// TestGetUsers_All returns two users for an unfiltered list request and
// asserts the success envelope carries both, with the MarshalJSON-injected
// password_set flag reflecting the pwd_hash column.
func TestGetUsers_All(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	rows := sqlmock.NewRows(userColumns).
		AddRow(1, time.Now(), time.Now(), nil, "a@x.com", "Aa", "A", "ACME", "admin", "active", nil, "hash").
		AddRow(2, time.Now(), time.Now(), nil, "b@x.com", "Bb", "B", "ACME", "user", "active", nil, nil)

	mock.ExpectQuery("SELECT \\* FROM `users`").WillReturnRows(rows)

	w := serve(http.MethodGet, "/admin/users", ac.GetUsers, "/admin/users", "", ctxValues{})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var env struct {
		Status string `json:"status"`
		Data   []struct {
			Email      string `json:"email"`
			Role       string `json:"role"`
			PasswordSet bool  `json:"password_set"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body %s)", err, w.Body.String())
	}
	if env.Status != "success" {
		t.Errorf("expected success, got %q", env.Status)
	}
	if len(env.Data) != 2 {
		t.Fatalf("expected 2 users, got %d: %+v", len(env.Data), env.Data)
	}
	if env.Data[0].Email != "a@x.com" || !env.Data[0].PasswordSet {
		t.Errorf("user[0] unexpected: %+v", env.Data[0])
	}
	if env.Data[1].Email != "b@x.com" || env.Data[1].PasswordSet {
		t.Errorf("user[1] unexpected (password_set should be false): %+v", env.Data[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGetUsers_FilteredByRoleAndStatus checks that the role and status query
// parameters are pushed into the WHERE clause as bound args, in the order the
// handler applies them (role then status).
func TestGetUsers_FilteredByRoleAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	rows := sqlmock.NewRows(userColumns).
		AddRow(1, time.Now(), time.Now(), nil, "a@x.com", "Aa", "A", "ACME", "admin", "active", nil, "hash")

	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `users`.`role` = \\? AND `users`.`status` = \\?").
		WithArgs("admin", "active").
		WillReturnRows(rows)

	w := serve(http.MethodGet, "/admin/users", ac.GetUsers,
		"/admin/users?role=admin&status=active", "", ctxValues{})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGetUsers_Empty verifies that a zero-row result maps to 404 (not an empty
// 200 list), as the handler special-cases len(users)==0.
func TestGetUsers_Empty(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users`").
		WillReturnRows(sqlmock.NewRows(userColumns))

	w := serve(http.MethodGet, "/admin/users", ac.GetUsers, "/admin/users", "", ctxValues{})

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
	}
	if env := decodeFail(t, w); env.Data.Error != "No users found that match the query" {
		t.Errorf("unexpected error message: %q", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGetUsers_BadQueryParam verifies that an out-of-enum query parameter is
// rejected at bind time (422) before any DB query runs.
func TestGetUsers_BadQueryParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	w := serve(http.MethodGet, "/admin/users", ac.GetUsers,
		"/admin/users?role=root", "", ctxValues{})

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusUnprocessableEntity, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetUserByEmail
// ---------------------------------------------------------------------------

// TestGetUserByEmail_Found looks up a user by the :email path param; the SELECT
// binds the email plus the LIMIT arg and returns one row.
func TestGetUserByEmail_Found(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	rows := sqlmock.NewRows(userColumns).
		AddRow(7, time.Now(), time.Now(), nil, "joe@me.com", "Doe", "Joe", "ACME", "user", "active", nil, "hash")

	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `users`.`email` = \\?").
		WithArgs("joe@me.com", 1).
		WillReturnRows(rows)

	w := serve(http.MethodGet, "/admin/users/:email", ac.GetUserByEmail,
		"/admin/users/joe@me.com", "", ctxValues{})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	var env struct {
		Status string `json:"status"`
		Data   struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body %s)", err, w.Body.String())
	}
	if env.Status != "success" || env.Data.Email != "joe@me.com" || env.Data.Role != "user" {
		t.Errorf("unexpected response: %+v", env)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGetUserByEmail_NotFound maps gorm.ErrRecordNotFound (no rows) to a 404.
func TestGetUserByEmail_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `users`.`email` = \\?").
		WithArgs("missing@me.com", 1).
		WillReturnRows(sqlmock.NewRows(userColumns))

	w := serve(http.MethodGet, "/admin/users/:email", ac.GetUserByEmail,
		"/admin/users/missing@me.com", "", ctxValues{})

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
	}
	if env := decodeFail(t, w); env.Data.Error != "User not found" {
		t.Errorf("unexpected error: %q", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteUserByEmail
// ---------------------------------------------------------------------------

// TestDeleteUserByEmail_Success loads the target by email, then soft-deletes
// it. The AfterDelete hook additionally hard-deletes the associated
// user_pwd_resets and user_email_changes rows, all inside one transaction.
func TestDeleteUserByEmail_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	rows := sqlmock.NewRows(userColumns).
		AddRow(9, time.Now(), time.Now(), nil, "target@me.com", "Doe", "Joe", "ACME", "user", "active", nil, "hash")
	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `users`.`email` = \\?").
		WithArgs("target@me.com", 1).
		WillReturnRows(rows)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET `deleted_at`").
		WithArgs(sqlmock.AnyArg(), 9).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `user_pwd_resets` WHERE user_id = \\?").
		WithArgs(9).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `user_email_changes` WHERE user_id = \\?").
		WithArgs(9).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	w := serve(http.MethodDelete, "/admin/users/:email", ac.DeleteUserByEmail,
		"/admin/users/target@me.com", "",
		ctxValues{userEmail: "admin@me.com", setEmail: true})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if env := decodeSuccess(t, w); env.Data.Message != "User deleted" {
		t.Errorf("unexpected message: %q", env.Data.Message)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestDeleteUserByEmail_NotFound returns no row for the lookup, so the handler
// short-circuits with 404 and never issues a delete.
func TestDeleteUserByEmail_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `users`.`email` = \\?").
		WithArgs("ghost@me.com", 1).
		WillReturnRows(sqlmock.NewRows(userColumns))

	w := serve(http.MethodDelete, "/admin/users/:email", ac.DeleteUserByEmail,
		"/admin/users/ghost@me.com", "",
		ctxValues{userEmail: "admin@me.com", setEmail: true})

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
	}
	if env := decodeFail(t, w); env.Data.Error != "User not found" {
		t.Errorf("unexpected error: %q", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestDeleteUserByEmail_Self rejects deleting one's own account with 403 before
// any DB access (the guard compares the path email to the caller's user_email).
func TestDeleteUserByEmail_Self(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	w := serve(http.MethodDelete, "/admin/users/:email", ac.DeleteUserByEmail,
		"/admin/users/admin@me.com", "",
		ctxValues{userEmail: "admin@me.com", setEmail: true})

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusForbidden, w.Code, w.Body.String())
	}
	if env := decodeFail(t, w); env.Data.Error != "Cannot delete own account" {
		t.Errorf("unexpected error: %q", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ChangeUserProfile
// ---------------------------------------------------------------------------

// TestChangeUserProfile_Success updates a different user's role and status; the
// handler loads the target, applies the changes and issues a Save (full-row
// UPDATE wrapped in a transaction).
func TestChangeUserProfile_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	rows := sqlmock.NewRows(userColumns).
		AddRow(2, time.Now(), time.Now(), nil, "target@me.com", "Doe", "Joe", "ACME", "user", "active", nil, "hash")
	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `users`.`email` = \\?").
		WithArgs("target@me.com", 1).
		WillReturnRows(rows)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `users` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := `{"role":"approver","status":"inactive"}`
	w := serve(http.MethodPatch, "/admin/users/:email", ac.ChangeUserProfile,
		"/admin/users/target@me.com", body,
		ctxValues{userID: 1, setID: true})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if env := decodeSuccess(t, w); env.Data.Message != "User profile updated" {
		t.Errorf("unexpected message: %q", env.Data.Message)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestChangeUserProfile_NoChanges verifies that once the target is loaded, an
// empty change set (no role, no status) is rejected with 400 and no Save runs.
func TestChangeUserProfile_NoChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	rows := sqlmock.NewRows(userColumns).
		AddRow(2, time.Now(), time.Now(), nil, "target@me.com", "Doe", "Joe", "ACME", "user", "active", nil, "hash")
	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `users`.`email` = \\?").
		WithArgs("target@me.com", 1).
		WillReturnRows(rows)

	w := serve(http.MethodPatch, "/admin/users/:email", ac.ChangeUserProfile,
		"/admin/users/target@me.com", `{}`,
		ctxValues{userID: 1, setID: true})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if env := decodeFail(t, w); env.Data.Error != "No changes requested" {
		t.Errorf("unexpected error: %q", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestChangeUserProfile_NotFound maps a missing target (no rows) to a 404
// before the change-set check.
func TestChangeUserProfile_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `users`.`email` = \\?").
		WithArgs("ghost@me.com", 1).
		WillReturnRows(sqlmock.NewRows(userColumns))

	w := serve(http.MethodPatch, "/admin/users/:email", ac.ChangeUserProfile,
		"/admin/users/ghost@me.com", `{"role":"user"}`,
		ctxValues{userID: 1, setID: true})

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
	}
	if env := decodeFail(t, w); env.Data.Error != "User not found" {
		t.Errorf("unexpected error: %q", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestChangeUserProfile_SelfLastAdmin covers changing one's own profile when
// one is the only active admin: after loading self (id == adminID) the handler
// counts active admins, finds exactly one, and returns 403 with the "only one
// admin left" message.
func TestChangeUserProfile_SelfLastAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	rows := sqlmock.NewRows(userColumns).
		AddRow(1, time.Now(), time.Now(), nil, "admin@me.com", "Doe", "Joe", "ACME", "admin", "active", nil, "hash")
	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `users`.`email` = \\?").
		WithArgs("admin@me.com", 1).
		WillReturnRows(rows)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WithArgs("admin", "active").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	w := serve(http.MethodPatch, "/admin/users/:email", ac.ChangeUserProfile,
		"/admin/users/admin@me.com", `{"status":"inactive"}`,
		ctxValues{userID: 1, setID: true})

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusForbidden, w.Code, w.Body.String())
	}
	env := decodeFail(t, w)
	if !strings.Contains(env.Data.Error, "only one admin left") {
		t.Errorf("unexpected error: %q", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestChangeUserProfile_SelfNotLastAdmin covers changing one's own profile when
// other active admins remain: the count returns >1 so the handler returns 403
// with the "Cannot change own role or status" message (still no Save).
func TestChangeUserProfile_SelfNotLastAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	rows := sqlmock.NewRows(userColumns).
		AddRow(1, time.Now(), time.Now(), nil, "admin@me.com", "Doe", "Joe", "ACME", "admin", "active", nil, "hash")
	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `users`.`email` = \\?").
		WithArgs("admin@me.com", 1).
		WillReturnRows(rows)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `users`").
		WithArgs("admin", "active").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	w := serve(http.MethodPatch, "/admin/users/:email", ac.ChangeUserProfile,
		"/admin/users/admin@me.com", `{"status":"inactive"}`,
		ctxValues{userID: 1, setID: true})

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusForbidden, w.Code, w.Body.String())
	}
	if env := decodeFail(t, w); env.Data.Error != "Cannot change own role or status" {
		t.Errorf("unexpected error: %q", env.Data.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestChangeUserProfile_BadRoleEnum verifies an out-of-enum role in the body is
// rejected at bind time (422) before any DB access.
func TestChangeUserProfile_BadRoleEnum(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ac, mock, cleanup := newAdminController(t)
	defer cleanup()

	w := serve(http.MethodPatch, "/admin/users/:email", ac.ChangeUserProfile,
		"/admin/users/target@me.com", `{"role":"root"}`,
		ctxValues{userID: 1, setID: true})

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusUnprocessableEntity, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction: %v", err)
	}
}
