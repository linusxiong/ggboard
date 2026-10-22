package guard_test

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"

	_ "github.com/linusxiong/ggboard/internal/subscription/formats"
)

// decrypt opens an armored age body with an AGE-SECRET-KEY-1… identity using the reference library.
func decrypt(t *testing.T, body []byte, identity string) []byte {
	t.Helper()
	id, err := age.ParseX25519Identity(identity)
	if err != nil {
		t.Fatal(err)
	}
	r, err := age.Decrypt(armor.NewReader(bytes.NewReader(body)), id)
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAgeKeyRoundTrip(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "alice@example.com")
	ctx := t.Context()

	if k, err := guard.LoadKey(ctx, a, u.ID); err != nil || k != nil {
		t.Fatalf("no key yet: %v %v", k, err)
	}
	if v := guard.View(a, nil, true); v.PublicKey != nil || v.Enabled || len(v.Formats) != 1 || v.Formats[0] != subscription.FormatClashMeta || !v.Optional || v.Strict {
		t.Fatalf("empty view = %+v", v)
	}
	k, priv, err := guard.GenerateKey(ctx, a, u.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if k.Enabled || !strings.HasPrefix(k.PublicKey, "age1") || !strings.HasPrefix(priv, "AGE-SECRET-KEY-1") || strings.Contains(k.PrivateKeyEnc, priv) {
		t.Fatalf("key = %+v", k)
	}
	if _, _, err := guard.GenerateKey(ctx, a, u.ID, false, nil); err != guard.ErrKeyExists { //nolint:errorlint // sentinel
		t.Fatalf("second generate = %v", err)
	}
	// The stored identity decrypts back for the owner and matches the public key.
	stored, err := guard.LoadKey(ctx, a, u.ID)
	if err != nil || stored == nil {
		t.Fatal(err)
	}
	v := guard.View(a, stored, true)
	if v.PrivateKey == nil || *v.PrivateKey != priv || *v.PublicKey != k.PublicKey {
		t.Fatalf("view = %+v", v)
	}
	if adm := guard.View(a, stored, false); adm.PrivateKey != nil {
		t.Fatal("admin view must not carry the private key")
	}
	id, err := age.ParseX25519Identity(*v.PrivateKey)
	if err != nil || id.Recipient().String() != k.PublicKey {
		t.Fatalf("identity/recipient mismatch: %v", err)
	}

	// Encrypt → armored text mihomo recognizes → decrypts with the stored private key.
	plain := []byte("mixed-port: 7890\nproxies: []\n")
	enc, err := guard.Encrypt(plain, k.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(enc, []byte("-----BEGIN AGE ENCRYPTED FILE-----\n")) || !bytes.HasSuffix(bytes.TrimSpace(enc), []byte("-----END AGE ENCRYPTED FILE-----")) {
		t.Fatalf("not armored:\n%s", enc)
	}
	if got := decrypt(t, enc, priv); !bytes.Equal(got, plain) {
		t.Fatalf("round trip = %q", got)
	}
	if _, err := guard.Encrypt(plain, "age1nope"); err == nil {
		t.Fatal("invalid recipient accepted")
	}

	// Regenerate keeps the switch; enable/disable; delete.
	if _, err := guard.SetKeyEnabled(ctx, a, u.ID, true); err != nil {
		t.Fatal(err)
	}
	k2, priv2, err := guard.GenerateKey(ctx, a, u.ID, true, nil)
	if err != nil || !k2.Enabled || k2.PublicKey == k.PublicKey || priv2 == priv {
		t.Fatalf("regenerate = %+v %v", k2, err)
	}
	if guard.KeyTag(k2.PublicKey) == guard.KeyTag(k.PublicKey) || len(guard.KeyTag(k.PublicKey)) != 8 {
		t.Fatal("key tags")
	}
	if err := guard.DeleteKey(ctx, a, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.SetKeyEnabled(ctx, a, u.ID, true); err != guard.ErrNoKey { //nolint:errorlint // sentinel
		t.Fatalf("enable without key = %v", err)
	}

	// A rotated jwt.secret leaves the public key usable and the private key unreadable.
	k3, _, err := guard.GenerateKey(ctx, a, u.ID, false, testutil.Ptr(true))
	if err != nil || !k3.Enabled {
		t.Fatal(err)
	}
	a.Cfg.JWT.Secret = "another-secret-another-secret-0123456789"
	if v := guard.View(a, k3, true); v.PrivateKey != nil || v.PublicKey == nil {
		t.Fatalf("view after rotation = %+v", v)
	}
}

func TestDecide(t *testing.T) {
	a := testutil.NewTestApp(t)
	on := &model.SubscriptionKey{UserID: 1, Enabled: true, PublicKey: "age1x"}
	off := &model.SubscriptionKey{UserID: 1, PublicKey: "age1x"}
	type tc struct {
		key              *model.SubscriptionKey
		format, ageParam string
		optional, strict bool
		want             guard.Decision
	}
	cases := []tc{
		{nil, "clash-meta", "", true, true, guard.ServePlain},
		{off, "clash-meta", "", true, true, guard.ServePlain},
		{on, "clash-meta", "", true, false, guard.ServeAge},
		{on, "clash-meta", "0", true, false, guard.ServePlain},
		{on, "clash-meta", "false", false, false, guard.ServeAge}, // ?age=0 not allowed
		{on, "clash-meta", "1", true, false, guard.ServeAge},
		{on, "v2ray", "", true, false, guard.ServePlain},
		{on, "stash", "", true, true, guard.RejectFormat},
		{on, "singbox", "0", true, true, guard.ServePlain}, // explicit plaintext wins while optional
		{on, "singbox", "0", false, true, guard.RejectFormat},
	}
	for i, c := range cases {
		if err := a.Settings.Set(map[string]any{"subscribe_age_optional": c.optional, "subscribe_age_strict": c.strict}); err != nil {
			t.Fatal(err)
		}
		if got := guard.Decide(a.Settings, c.key, c.format, c.ageParam); got != c.want {
			t.Errorf("case %d: %v, want %v", i, got, c.want)
		}
	}
	if !subscription.SupportsAge(subscription.FormatClashMeta) || subscription.SupportsAge(subscription.FormatStash) ||
		subscription.SupportsAge(subscription.FormatClash) || subscription.SupportsAge("nope") {
		t.Fatal("format flags")
	}
}

func record(a *app.App, uid uint64, token, ip string, at time.Time) {
	guard.Record(a, guard.Access{UserID: uid, Token: token, IP: ip, UserAgent: "clash-verge/v2.2.3 (mihomo)", Format: "clash-meta", At: at})
}

func TestAccessLogWritesCapsAndMasks(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "alice@example.com")
	other := testutil.CreateUser(t, a, "bob@example.com")
	ctx := t.Context()
	now := time.Now()

	record(a, u.ID, u.Token, "203.0.113.7", now.Add(-31*24*time.Hour)) // older than 30 days
	for i := range 130 {
		record(a, u.ID, u.Token, "::ffff:198.51.100.9", now.Add(time.Duration(i-130)*time.Minute))
	}
	record(a, u.ID, u.Token, "2001:db8:1:2:aaaa:bbbb:cccc:dddd", now)
	record(a, other.ID, other.Token, "192.0.2.1", now)
	if guard.Pending(a) != 133 {
		t.Fatalf("pending = %d", guard.Pending(a))
	}
	if err := guard.FlushAccess(ctx, a); err != nil {
		t.Fatal(err)
	}
	if guard.Pending(a) != 0 {
		t.Fatal("flush left rows behind")
	}
	var n int64
	a.DB.Model(&model.SubscriptionAccess{}).Count(&n)
	if n != 133 {
		t.Fatalf("rows = %d", n)
	}

	// User view: newest first, masked, limited to 50.
	list, err := guard.ListAccess(ctx, a, u.ID, guard.UserLogLimit, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 50 || list[0].IP != "2001:db8:1:2:*" || list[1].IP != "198.51.100.*" || list[0].IPBucket != "" ||
		list[0].Client != "clash-verge/v2.2.3" || list[0].Format != "clash-meta" || list[0].TokenTag != "" {
		t.Fatalf("user list = %+v", list[:2])
	}
	// Admin view: unmasked, with the /64 bucket and the token tag.
	full, err := guard.ListAccess(ctx, a, u.ID, guard.AdminLogLimit, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 100 || full[0].IP != "2001:db8:1:2:aaaa:bbbb:cccc:dddd" || full[0].IPBucket != "2001:db8:1:2::/64" ||
		full[1].IP != "198.51.100.9" || full[0].TokenTag != guard.TokenTag(u.Token) {
		t.Fatalf("admin list = %+v", full[:2])
	}

	// Prune: 30 days and the newest 100 per user.
	if err := guard.PruneAccess(ctx, a, now); err != nil {
		t.Fatal(err)
	}
	a.DB.Model(&model.SubscriptionAccess{}).Where("user_id = ?", u.ID).Count(&n)
	if n != guard.KeepPerUser {
		t.Fatalf("rows after prune = %d", n)
	}
	a.DB.Model(&model.SubscriptionAccess{}).Where("user_id = ?", other.ID).Count(&n)
	if n != 1 {
		t.Fatalf("other user's rows = %d", n)
	}
	var oldest model.SubscriptionAccess
	a.DB.Where("user_id = ?", u.ID).Order("id").First(&oldest)
	if oldest.At.Before(now.Add(-24 * time.Hour)) {
		t.Fatalf("oldest kept row = %v", oldest.At)
	}

	// The prune job is registered and runs through the scheduler.
	if err := a.Scheduler.RunNow(guard.JobAccessPrune); err != nil {
		t.Fatal(err)
	}
	if err := a.Scheduler.RunNow(guard.JobAccessFlush); err != nil {
		t.Fatal(err)
	}
}

func TestAccessLogNeverBlocks(t *testing.T) {
	a := testutil.NewTestApp(t)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 1500 {
				record(a, uint64(g+1), "t", "192.0.2."+string(rune('0'+i%10)), time.Now())
			}
		}()
	}
	wg.Wait()
	if p := guard.Pending(a); p != guard.MaxBuffered {
		t.Fatalf("pending = %d, want the cap %d", p, guard.MaxBuffered)
	}
}

func TestMaskAndFamily(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.7": "203.0.113.*", "::ffff:203.0.113.7": "203.0.113.*", "2001:db8::1": "2001:db8:0:0:*", "x": "*",
	} {
		if got := guard.MaskIP(in); got != want {
			t.Errorf("MaskIP(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"": "unknown", "clash.meta/1.19.27": "clash.meta/1.19.27", "Mozilla/5.0 (X11)": "Mozilla/5.0",
		"Quantumult%20X/1.4.1": "Quantumult%20X/1.4.1", "Surge iOS/3000": "Surge", strings.Repeat("a", 100): strings.Repeat("a", 64),
	} {
		if got := guard.ClientFamily(in); got != want {
			t.Errorf("ClientFamily(%q) = %q, want %q", in, got, want)
		}
	}
	if len(guard.TokenTag("abc")) != 8 || guard.TokenTag("abc") == guard.TokenTag("abd") {
		t.Fatal("token tag")
	}
}

type leakCall struct {
	userID   uint64
	token    string
	distinct int
}

// hookCalls replaces subscription.LeakHook for the test and returns the recorded calls.
func hookCalls(t *testing.T) *[]leakCall {
	t.Helper()
	var mu sync.Mutex
	calls := &[]leakCall{}
	prev := subscription.LeakHook
	subscription.LeakHook = func(_ *app.App, u *model.User, distinct int) {
		mu.Lock()
		*calls = append(*calls, leakCall{u.ID, u.Token, distinct})
		mu.Unlock()
	}
	t.Cleanup(func() { subscription.LeakHook = prev })
	return calls
}

func TestLeakDetectionNotify(t *testing.T) {
	calls := hookCalls(t)
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"subscribe_leak_max_ips": 3}))
	u := testutil.CreateUser(t, a, "alice@example.com")
	ctx := t.Context()
	now := time.Now()

	// Same /64 counts once; an old access outside the window does not count.
	record(a, u.ID, u.Token, "2001:db8:1:2::1", now)
	record(a, u.ID, u.Token, "2001:db8:1:2::2", now)
	record(a, u.ID, u.Token, "192.0.2.1", now)
	record(a, u.ID, u.Token, "192.0.2.2", now)
	record(a, u.ID, u.Token, "192.0.2.3", now.Add(-25*time.Hour))
	if err := guard.FlushAccess(ctx, a); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatalf("3 buckets must not alert: %+v", *calls)
	}
	record(a, u.ID, u.Token, "192.0.2.4", now)
	if err := guard.FlushAccess(ctx, a); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].distinct != 4 || (*calls)[0].token != u.Token {
		t.Fatalf("calls = %+v", *calls)
	}
	// Once per token and window.
	record(a, u.ID, u.Token, "192.0.2.5", now)
	if err := guard.FlushAccess(ctx, a); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("repeated alert: %+v", *calls)
	}
	var fresh model.User
	a.DB.First(&fresh, u.ID)
	if fresh.Token != u.Token {
		t.Fatal("notify must not rotate the token")
	}
}

func TestLeakDetectionRotateAndDefaultHook(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{
		"subscribe_leak_max_ips": 1, "subscribe_leak_action": "rotate", "app_url": "https://panel.example.com",
	}))
	u := testutil.CreateUser(t, a, "alice@example.com")
	ctx := t.Context()
	record(a, u.ID, u.Token, "192.0.2.1", time.Now())
	record(a, u.ID, u.Token, "198.51.100.1", time.Now())
	if err := guard.FlushAccess(ctx, a); err != nil {
		t.Fatal(err)
	}
	var fresh model.User
	a.DB.First(&fresh, u.ID)
	if fresh.Token == u.Token || fresh.UUID != u.UUID || fresh.PasswordVersion != u.PasswordVersion {
		t.Fatalf("token not rotated alone: %+v", fresh)
	}
	mails := testutil.Mails(t, a)
	if len(mails) != 1 || mails[0].To != u.Email || !strings.Contains(mails[0].Text, "token="+fresh.Token) ||
		!strings.Contains(mails[0].Text, "2 个不同的 IP") || !strings.Contains(mails[0].Text, "https://panel.example.com/subscription") {
		t.Fatalf("mails = %+v", mails)
	}

	// The new token starts a fresh window (its rows carry another token tag).
	record(a, u.ID, fresh.Token, "203.0.113.1", time.Now())
	if err := guard.FlushAccess(ctx, a); err != nil {
		t.Fatal(err)
	}
	var again model.User
	a.DB.First(&again, u.ID)
	if again.Token != fresh.Token {
		t.Fatal("rotated again although the new token was used from one IP")
	}
}
