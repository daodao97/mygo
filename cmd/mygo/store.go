package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// mygo publish uploads what mygo build made for phones to their stores: the
// .ipa to App Store Connect with Xcode's altool, the .aab to a track of
// Google Play through its REST API. (mygo build -upload is apart: it uploads
// the desktop's installers and updates to where updates point to.)

func runPublish(args []string) error {
	flags := newFlags("publish", "[flags] [dir]", `Uploads a build of mygo build -platform ios or android to its store.

iOS uploads the .ipa to App Store Connect with an API key of the team
(App Store Connect › Users and Access › Integrations): its ID in
MYGO_ASC_KEY_ID, its issuer in MYGO_ASC_ISSUER_ID and the path of its .p8 in
MYGO_ASC_KEY. It then shows in TestFlight, once processed.

Android uploads the .aab to a track of Google Play with a service account
that has access to the app in Play Console: the path of its JSON key, or the
JSON itself, in MYGO_PLAY_SERVICE_ACCOUNT. Google Play takes the first upload
of an app in its console only.`)
	platform := flags.String("platform", "", "the store: ios (App Store Connect) or android (Google Play)")
	file := flags.String("file", "", "the file to upload (default: what mygo build wrote for this version)")
	track := flags.String("track", "internal", "with android, the track: internal, alpha, beta, production or a closed testing track's name")
	status := flags.String("status", "completed", "with android, the release's status: completed, or draft for an app Google Play has not reviewed yet")
	if err := flags.Parse(args); err != nil {
		return err
	}
	c, err := loadConfig(dirArg(flags.Args()))
	if err != nil {
		return err
	}
	switch *platform {
	case "ios":
		return publishAppStore(c, *file)
	case "android":
		if *status != "completed" && *status != "draft" {
			return fmt.Errorf("unknown status %q: completed or draft", *status)
		}
		return publishGooglePlay(c, *file, *track, *status)
	case "":
		flags.Usage()
		return errors.New("publish needs -platform ios or android")
	}
	return fmt.Errorf("unknown platform %q: publish takes ios or android", *platform)
}

// storeArtifact returns the file to upload: file if given, else what mygo
// build wrote for this version.
func storeArtifact(c *Config, file, platformDir, ext, build string) (string, error) {
	path := c.path(filepath.Join(c.Out, platformDir, c.slug()+"-"+c.Version+ext))
	if file != "" {
		path = file
		if !filepath.IsAbs(path) {
			if wd, err := os.Getwd(); err == nil {
				path = filepath.Join(wd, path)
			}
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		if file != "" {
			return "", err
		}
		return "", fmt.Errorf("no %s: run %s first", path, build)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	if !strings.EqualFold(filepath.Ext(path), ext) {
		return "", fmt.Errorf("%s is not %s file", path, map[string]string{".ipa": "an .ipa", ".aab": "an .aab"}[ext])
	}
	return path, nil
}

// App Store Connect.

// ascCredentials is an API key of App Store Connect.
type ascCredentials struct {
	keyID, issuerID, keyPath string
}

// ascFromEnv reads the key's variables, naming what is missing.
func ascFromEnv() (ascCredentials, error) {
	k := ascCredentials{os.Getenv("MYGO_ASC_KEY_ID"), os.Getenv("MYGO_ASC_ISSUER_ID"), os.Getenv("MYGO_ASC_KEY")}
	var missing []string
	for name, v := range map[string]string{"MYGO_ASC_KEY_ID": k.keyID, "MYGO_ASC_ISSUER_ID": k.issuerID, "MYGO_ASC_KEY": k.keyPath} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return k, fmt.Errorf("uploading to App Store Connect needs an API key: set %s", strings.Join(missing, ", "))
	}
	if !ascKeyID.MatchString(k.keyID) {
		return k, fmt.Errorf("MYGO_ASC_KEY_ID %q is not the ID of a key", k.keyID)
	}
	return k, nil
}

// ascKeyID keeps the key's ID a file name: altool reads AuthKey_<id>.p8.
var ascKeyID = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// altoolUploadArgs are the arguments of xcrun that upload an .ipa.
func altoolUploadArgs(ipa string, k ascCredentials) []string {
	return []string{"altool", "--upload-app", "-f", ipa, "-t", "ios", "--apiKey", k.keyID, "--apiIssuer", k.issuerID, "--output-format", "normal"}
}

// ascKeyDir copies the key to a directory of its own, under the name altool
// looks for in API_PRIVATE_KEYS_DIR.
func ascKeyDir(k ascCredentials) (string, error) {
	data, err := os.ReadFile(k.keyPath)
	if err != nil {
		return "", fmt.Errorf("MYGO_ASC_KEY: %w", err)
	}
	dir, err := os.MkdirTemp("", "mygo-asc-")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "AuthKey_"+k.keyID+".p8"), data, 0o600); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// publishAppStore uploads the .ipa with altool.
func publishAppStore(c *Config, file string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("uploading to App Store Connect takes a Mac with Xcode")
	}
	ipa, err := storeArtifact(c, file, "ios-arm64", ".ipa", "mygo build -platform ios")
	if err != nil {
		return err
	}
	k, err := ascFromEnv()
	if err != nil {
		return err
	}
	dir, err := ascKeyDir(k)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	t := con.start("Uploading " + filepath.Base(ipa) + " to App Store Connect")
	t.set(sizeOf(ipa))
	cmd := exec.Command("xcrun", altoolUploadArgs(ipa, k)...)
	cmd.Env = append(os.Environ(), "API_PRIVATE_KEYS_DIR="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("altool: %v\n%s", err, bytes.TrimSpace(out))
	}
	if err := t.end(err, "Uploaded "+filepath.Base(ipa)+" to App Store Connect"); err != nil {
		return err
	}
	logf("It shows in TestFlight once App Store Connect has processed it")
	return nil
}

// Google Play.

// playServiceAccount is what a service account's JSON key holds.
type playServiceAccount struct {
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	PrivateKeyID string `json:"private_key_id"`
	TokenURI     string `json:"token_uri"`

	key *rsa.PrivateKey
}

const (
	playScope     = "https://www.googleapis.com/auth/androidpublisher"
	playBase      = "https://androidpublisher.googleapis.com"
	googleTokens  = "https://oauth2.googleapis.com/token"
	jwtGrantType  = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	tokenLifetime = time.Hour
)

// parsePlayServiceAccount reads a service account's JSON key and its private
// key.
func parsePlayServiceAccount(data []byte) (*playServiceAccount, error) {
	var a playServiceAccount
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("MYGO_PLAY_SERVICE_ACCOUNT: %w", err)
	}
	if a.ClientEmail == "" || a.PrivateKey == "" {
		return nil, errors.New("MYGO_PLAY_SERVICE_ACCOUNT has no client_email or private_key: it is the JSON key of a service account")
	}
	if a.TokenURI == "" {
		a.TokenURI = googleTokens
	}
	block, _ := pem.Decode([]byte(a.PrivateKey))
	if block == nil {
		return nil, errors.New("MYGO_PLAY_SERVICE_ACCOUNT: its private_key is not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		rsaKey, err2 := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("MYGO_PLAY_SERVICE_ACCOUNT: its private_key: %w", err)
		}
		key = rsaKey
	}
	var ok bool
	if a.key, ok = key.(*rsa.PrivateKey); !ok {
		return nil, errors.New("MYGO_PLAY_SERVICE_ACCOUNT: its private_key is not an RSA key")
	}
	return &a, nil
}

// assertion is the JWT that asks Google for a token, signed with RS256.
func (a *playServiceAccount) assertion(now time.Time) (string, error) {
	enc := base64.RawURLEncoding
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": a.PrivateKeyID})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{
		"iss":   a.ClientEmail,
		"scope": playScope,
		"aud":   a.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(tokenLifetime).Unix(),
	})
	if err != nil {
		return "", err
	}
	signed := enc.EncodeToString(header) + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signed + "." + enc.EncodeToString(sig), nil
}

// playClient calls Google Play's Android Publisher API for an app.
type playClient struct {
	http    *http.Client
	base    string // playBase, or a test's server
	account *playServiceAccount
	pkg     string
	now     func() time.Time
	token   string
}

// playError is an answer of the API other than success.
type playError struct {
	action string
	status int
	msg    string
}

func (e *playError) Error() string {
	return fmt.Sprintf("Google Play: %s: %d %s", e.action, e.status, e.msg)
}

// playAnswerError reads the error of an answer.
func playAnswerError(action string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var doc struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		// The token endpoint's.
		Description string `json:"error_description"`
	}
	msg := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &doc) == nil {
		msg = or(doc.Error.Message, or(doc.Description, msg))
	}
	return &playError{action, resp.StatusCode, msg}
}

// authenticate trades the assertion for a token.
func (p *playClient) authenticate(ctx context.Context) error {
	assertion, err := p.account.assertion(p.now())
	if err != nil {
		return err
	}
	form := url.Values{"grant_type": {jwtGrantType}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return playAnswerError("signing in as "+p.account.ClientEmail, resp)
	}
	var doc struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return err
	}
	if doc.AccessToken == "" {
		return errors.New("Google Play: signing in gave no token")
	}
	p.token = doc.AccessToken
	return nil
}

// call makes a request of the API and decodes its answer into out, if any.
func (p *playClient) call(ctx context.Context, action, method, path string, header http.Header, body io.Reader, size int64, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Authorization", "Bearer "+p.token)
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return playAnswerError(action, resp)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// callJSON makes a request with a JSON body, or none.
func (p *playClient) callJSON(ctx context.Context, action, method, path string, in, out any) error {
	var body io.Reader
	var size int64
	header := http.Header{}
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body, size = bytes.NewReader(data), int64(len(data))
		header.Set("Content-Type", "application/json")
	}
	return p.call(ctx, action, method, path, header, body, size, out)
}

func (p *playClient) editPath(edit string) string {
	s := "/androidpublisher/v3/applications/" + url.PathEscape(p.pkg) + "/edits"
	if edit != "" {
		s += "/" + url.PathEscape(edit)
	}
	return s
}

// playRelease is what an upload makes.
type playRelease struct {
	file, track, status, name string
}

// upload makes an edit of the app, uploads the bundle to it, puts it on the
// track and commits, returning the bundle's version code. A failure deletes
// the edit, which would otherwise stay until it expires.
func (p *playClient) upload(ctx context.Context, rel playRelease, progress func(string)) (versionCode int64, err error) {
	progress("signing in")
	if err := p.authenticate(ctx); err != nil {
		return 0, err
	}
	var edit struct {
		ID string `json:"id"`
	}
	progress("starting an edit")
	if err := p.callJSON(ctx, "starting an edit of "+p.pkg, http.MethodPost, p.editPath(""), map[string]any{}, &edit); err != nil {
		return 0, err
	}
	if edit.ID == "" {
		return 0, errors.New("Google Play: starting an edit gave none")
	}
	defer func() {
		if err != nil {
			// With its own time: ctx may be why it failed.
			cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			p.callJSON(cleanup, "deleting the edit", http.MethodDelete, p.editPath(edit.ID), nil, nil)
		}
	}()

	progress("uploading " + sizeOf(rel.file))
	if versionCode, err = p.uploadBundle(ctx, edit.ID, rel.file); err != nil {
		return 0, err
	}
	progress("putting it on the track " + rel.track)
	code := strconv.FormatInt(versionCode, 10)
	track := map[string]any{
		"track": rel.track,
		"releases": []any{map[string]any{
			"name":         rel.name,
			"versionCodes": []string{code},
			"status":       rel.status,
		}},
	}
	path := p.editPath(edit.ID) + "/tracks/" + url.PathEscape(rel.track)
	if err = p.callJSON(ctx, "putting the bundle on the track "+rel.track, http.MethodPut, path, track, nil); err != nil {
		return 0, err
	}
	progress("committing")
	if err = p.callJSON(ctx, "committing the edit", http.MethodPost, p.editPath(edit.ID)+":commit", nil, nil); err != nil {
		return 0, err
	}
	return versionCode, nil
}

// uploadBundle uploads the file to the edit, checking what arrived by its
// hash, and returns its version code.
func (p *playClient) uploadBundle(ctx context.Context, edit, file string) (int64, error) {
	f, err := os.Open(file)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	var bundle struct {
		VersionCode int64  `json:"versionCode"`
		SHA256      string `json:"sha256"`
	}
	path := "/upload" + p.editPath(edit) + "/bundles?uploadType=media"
	header := http.Header{"Content-Type": {"application/octet-stream"}}
	if err := p.call(ctx, "uploading "+filepath.Base(file), http.MethodPost, path, header, f, info.Size(), &bundle); err != nil {
		return 0, err
	}
	if want := hex.EncodeToString(h.Sum(nil)); bundle.SHA256 != "" && !strings.EqualFold(bundle.SHA256, want) {
		return 0, fmt.Errorf("Google Play: the upload of %s arrived with another hash", filepath.Base(file))
	}
	if bundle.VersionCode == 0 {
		return 0, errors.New("Google Play: the upload gave no version code")
	}
	return bundle.VersionCode, nil
}

// readPlayServiceAccount reads MYGO_PLAY_SERVICE_ACCOUNT: a path, or the
// JSON itself, as secrets of CI often are.
func readPlayServiceAccount() (*playServiceAccount, error) {
	v := os.Getenv("MYGO_PLAY_SERVICE_ACCOUNT")
	if v == "" {
		return nil, errors.New("uploading to Google Play needs a service account: set MYGO_PLAY_SERVICE_ACCOUNT to the path of its JSON key")
	}
	data := []byte(v)
	if !strings.HasPrefix(strings.TrimSpace(v), "{") {
		var err error
		if data, err = os.ReadFile(v); err != nil {
			return nil, fmt.Errorf("MYGO_PLAY_SERVICE_ACCOUNT: %w", err)
		}
	}
	return parsePlayServiceAccount(data)
}

// publishGooglePlay uploads the .aab to a track of the app.
func publishGooglePlay(c *Config, file, track, status string) error {
	aab, err := storeArtifact(c, file, "android", ".aab", "mygo build -platform android")
	if err != nil {
		return err
	}
	account, err := readPlayServiceAccount()
	if err != nil {
		return err
	}
	if c.Android.Keystore == "" && file == "" {
		warnf("android.keystore is not set: Google Play refuses a bundle signed with the debug key")
	}
	client := &playClient{http: &http.Client{Timeout: 30 * time.Minute}, base: playBase, account: account, pkg: c.Identifier, now: time.Now}
	t := con.start("Uploading " + filepath.Base(aab) + " to Google Play")
	code, err := client.upload(context.Background(), playRelease{file: aab, track: track, status: status, name: c.Version}, t.set)
	if err := t.end(err, "Uploaded "+filepath.Base(aab)+" to Google Play"); err != nil {
		return err
	}
	logf("Version code %d is on the track %s of %s", code, track, c.Identifier)
	return nil
}
