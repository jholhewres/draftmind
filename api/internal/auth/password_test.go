package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordHashIsArgon2idWithItsParameters(t *testing.T) {
	hash, err := hashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") || strings.Contains(hash, "correct horse") {
		t.Fatalf("hash %q", hash)
	}
	if again, _ := hashPassword("correct horse"); again == hash {
		t.Fatal("two hashes of one password are equal: no salt")
	}
	if !checkPassword(hash, "correct horse") {
		t.Fatal("the right password does not match")
	}
	for _, wrong := range []string{"correct horsf", "", "$argon2id$"} {
		if checkPassword(hash, wrong) {
			t.Fatalf("%q matches", wrong)
		}
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=x$a$b", "$argon2i$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA"} {
		if checkPassword(bad, "correct horse") {
			t.Fatalf("malformed hash %q matches", bad)
		}
	}
}

func (h *harness) register(email, password string) *httptest.ResponseRecorder {
	return h.do(http.MethodPost, "/api/auth/register", `{"email": "`+email+`", "password": "`+password+`"}`)
}

func (h *harness) login(email, password string) *httptest.ResponseRecorder {
	return h.do(http.MethodPost, "/api/auth/login", `{"email": "`+email+`", "password": "`+password+`"}`)
}

func TestAVisitorRegistersThenSignsInAndOutWithAPassword(t *testing.T) {
	h := newHarness(t)

	if rec := h.register("player@example.com", "1234567"); rec.Code != http.StatusBadRequest {
		t.Fatalf("7 characters: status %d: %s", rec.Code, rec.Body)
	}
	if rec := h.register("player@example.com", strings.Repeat("a", 201)); rec.Code != http.StatusBadRequest {
		t.Fatalf("201 characters: status %d: %s", rec.Code, rec.Body)
	}
	created := h.register("  Player@Example.com ", "12345678")
	if created.Code != http.StatusOK || sessionCookie(created) != nil {
		t.Fatalf("register: status %d, cookie %v: %s", created.Code, sessionCookie(created), created.Body)
	}

	var stored string
	if err := h.svc.pool.QueryRow(context.Background(), `SELECT password_hash FROM app_user WHERE email = 'player@example.com'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "$argon2id$") || strings.Contains(stored, "12345678") {
		t.Fatalf("stored %q", stored)
	}

	in := h.login("PLAYER@example.com", "12345678")
	cookie := sessionCookie(in)
	if in.Code != http.StatusOK || cookie == nil {
		t.Fatalf("login: status %d: %s", in.Code, in.Body)
	}
	if me := h.do(http.MethodGet, "/api/me", "", cookie); me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"email":"player@example.com"`) {
		t.Fatalf("me: status %d: %s", me.Code, me.Body)
	}
	if out := h.do(http.MethodPost, "/api/auth/signout", "", cookie); out.Code != http.StatusNoContent {
		t.Fatalf("signout: status %d", out.Code)
	}
	if me := h.do(http.MethodGet, "/api/me", "", cookie); me.Code != http.StatusUnauthorized {
		t.Fatalf("me after signout: status %d", me.Code)
	}

	// Registering a taken address answers as a new one and changes nothing.
	again := h.register("player@example.com", "another password")
	if again.Code != created.Code || again.Body.String() != created.Body.String() {
		t.Fatalf("taken address: %d %s, new address: %d %s", again.Code, again.Body, created.Code, created.Body)
	}
	if rec := h.login("player@example.com", "another password"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("the second register set the password: status %d", rec.Code)
	}

	// An account from a magic link has no password and cannot get one by register.
	if rec := h.openLink(h.requestLink("linked@example.com")); rec.Code != http.StatusOK {
		t.Fatalf("magic link: status %d", rec.Code)
	}
	if rec := h.register("linked@example.com", "12345678"); rec.Code != http.StatusOK {
		t.Fatalf("register over a link account: status %d", rec.Code)
	}
	if rec := h.login("linked@example.com", "12345678"); rec.Code != http.StatusUnauthorized || sessionCookie(rec) != nil {
		t.Fatalf("register took over the link account: status %d", rec.Code)
	}
}

func TestAWrongPasswordAndAnUnknownEmailGetTheSameError(t *testing.T) {
	h := newHarness(t)
	if rec := h.register("player@example.com", "12345678"); rec.Code != http.StatusOK {
		t.Fatalf("register: status %d", rec.Code)
	}
	wrong := h.login("player@example.com", "87654321")
	unknown := h.login("nobody@example.com", "12345678")
	short := h.login("player@example.com", "1")
	for name, rec := range map[string]*httptest.ResponseRecorder{"unknown e-mail": unknown, "short password": short} {
		if rec.Code != wrong.Code || rec.Body.String() != wrong.Body.String() {
			t.Fatalf("%s: %d %s; wrong password: %d %s", name, rec.Code, rec.Body, wrong.Code, wrong.Body)
		}
	}
	if wrong.Code != http.StatusUnauthorized || sessionCookie(wrong) != nil {
		t.Fatalf("wrong password: status %d", wrong.Code)
	}
}

func TestMethodsSayWhetherEmailLinksAreOn(t *testing.T) {
	h := newHarness(t)
	if rec := h.do(http.MethodGet, "/api/auth/methods", ""); rec.Code != http.StatusOK || rec.Body.String() != `{"password":true,"steam":true,"emailLink":true}`+"\n" {
		t.Fatalf("with a mailer: %d %s", rec.Code, rec.Body)
	}
	h.svc.WithMailer(nil)
	if rec := h.do(http.MethodGet, "/api/auth/methods", ""); rec.Body.String() != `{"password":true,"steam":true,"emailLink":false}`+"\n" {
		t.Fatalf("without a mailer: %d %s", rec.Code, rec.Body)
	}
}

func TestAMagicLinkDropsAPasswordSetWithoutProofOfTheAddress(t *testing.T) {
	h := newHarness(t)
	// Someone registers an address that is not theirs and signs in.
	if rec := h.register("victim@example.com", "attacker-pass"); rec.Code != http.StatusOK {
		t.Fatalf("register: status %d", rec.Code)
	}
	squatter := sessionCookie(h.login("victim@example.com", "attacker-pass"))
	if squatter == nil {
		t.Fatal("login set no cookie")
	}
	// The owner proves the address with a link: the squatter is out.
	if rec := h.openLink(h.requestLink("victim@example.com")); rec.Code != http.StatusOK {
		t.Fatalf("magic link: status %d", rec.Code)
	}
	if me := h.do(http.MethodGet, "/api/me", "", squatter); me.Code != http.StatusUnauthorized {
		t.Fatalf("the squatter's session outlived the owner's link: status %d", me.Code)
	}
	if rec := h.login("victim@example.com", "attacker-pass"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("the squatter's password still works: status %d", rec.Code)
	}
}
