package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

var logger = NewColoredLogger("", nil)

// loadEnvFromDotfile merges .env into the environment when the file exists.
// On Fly, use `fly secrets set` / [env]; there is no .env file in the image.
func loadEnvFromDotfile() {
	_ = godotenv.Load(".env")
}

// wordPressContentFromRaw turns the body lines after "Image:" (plain text / newlines)
// into Gutenberg blocks, matching the legacy execPosts behavior.
func wordPressContentFromRaw(raw string) string {
	var lines []string
	for _, l := range strings.Split(raw, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	contentCount := 0
	for _, line := range lines {
		if contentCount == 0 {
			b.WriteString(fmt.Sprintf(`<!-- wp:paragraph --><p>%s</p><!-- /wp:paragraph -->`, line))
			contentCount = 1
			continue
		}
		if contentCount == 2 {
			b.WriteString(fmt.Sprintf(`<!-- wp:paragraph --><p><a href="%s">Ver nota completa</a></p><!-- /wp:paragraph -->`, line))
			contentCount = 3
			continue
		}
		b.WriteString(fmt.Sprintf(`<!-- wp:paragraph --><p>%s</p><!-- /wp:paragraph -->`, line))
		contentCount++
	}
	return b.String()
}

func preparePostsForWP(posts []Post) []Post {
	out := make([]Post, len(posts))
	for i := range posts {
		out[i] = posts[i]
		out[i].Title = strings.TrimSpace(posts[i].Title)
		out[i].Category = strings.TrimSpace(posts[i].Category)
		out[i].Image = strings.TrimSpace(posts[i].Image)
		out[i].Content = wordPressContentFromRaw(posts[i].Content)
	}
	return out
}

func uploadPosts(startIndex int) {
	loadEnvFromDotfile()

	posts, err := parsePosts("posts.txt")
	if err != nil {
		logger.Error("Error reading posts: %v", err)
		return
	}

	posts = preparePostsForWP(posts)

	if startIndex >= len(posts) {
		logger.Error("Start index %d is out of range (total posts: %d)", startIndex, len(posts))
		return
	}

	logger.Info("Total posts to upload: %d", len(posts))
	if startIndex > 0 {
		logger.Info("Resuming from post %d", startIndex)
	}

	token := getJWTToken()

	for i := startIndex; i < len(posts); i++ {
		post := posts[i]
		categoryID := getCategoryID(post.Category)
		imageID, err := uploadFeaturedImage(post.Image, i, token)
		if err != nil {
			logger.Error("%v. Resume with: go run . upload %d", err, i)
			panic(err.Error())
		}
		if err := createPost(post.Title, post.Content, categoryID, imageID, i, token); err != nil {
			logger.Error("%v. Resume with: go run . upload %d", err, i)
			panic(err.Error())
		}

		logger.Info("[%d/%d] %s", i+1, len(posts), post.Title)
	}
}

// UploadPostsFromEditor uploads posts parsed the same way as posts.txt (raw Content body).
// Returns the 0-based index of the failed post and an error if any step fails.
func UploadPostsFromEditor(startIndex int, posts []Post) (failedAt int, err error) {
	loadEnvFromDotfile()
	prepared := preparePostsForWP(posts)
	if len(prepared) == 0 {
		return 0, fmt.Errorf("no posts to upload")
	}
	if startIndex < 0 || startIndex >= len(prepared) {
		return 0, fmt.Errorf("start index %d out of range (posts: %d)", startIndex, len(prepared))
	}

	token, err := getJWTTokenE()
	if err != nil {
		return 0, err
	}

	for i := startIndex; i < len(prepared); i++ {
		post := prepared[i]
		categoryID := getCategoryID(post.Category)
		imageID, err := uploadFeaturedImage(post.Image, i, token)
		if err != nil {
			return i, fmt.Errorf("post %d (%q): featured image: %w", i, post.Title, err)
		}
		if err := createPost(post.Title, post.Content, categoryID, imageID, i, token); err != nil {
			return i, fmt.Errorf("post %d (%q): create post: %w", i, post.Title, err)
		}
		logger.Info("[%d/%d] %s", i+1, len(prepared), post.Title)
	}
	return -1, nil
}

func runUploadOnly(startIndex int) {
	uploadPosts(startIndex)
	logger.Info("Upload complete!")
}

func getJWTTokenE() (string, error) {
	tokenURL := "https://gen.boletindiario.in/wp-json/jwt-auth/v1/token"
	form := fmt.Sprintf("username=%s&password=%s", os.Getenv("EMAIL"), os.Getenv("PASSWORD"))

	var res *http.Response
	err := withHTTPRetry(func() error {
		req, reqErr := http.NewRequest("POST", tokenURL, strings.NewReader(form))
		if reqErr != nil {
			return reqErr
		}
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		var doErr error
		res, doErr = wordPressHTTPClient().Do(req)
		return doErr
	})
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return "", err
	}

	if token, ok := result["token"]; ok && token != nil {
		s, _ := token.(string)
		if s != "" {
			return s, nil
		}
	}

	if errorMsg, ok := result["message"]; ok {
		return "", fmt.Errorf("%v", errorMsg)
	}

	return "", fmt.Errorf("no token received")
}

func getJWTToken() string {
	t, err := getJWTTokenE()
	if err != nil {
		logger.Error("JWT Authentication failed: %v", err)
		panic(err.Error())
	}
	return t
}

func getCategoryID(childSlug string) int {
	var parentSlug string
	if childSlug == "menciones-icpnl" {
		parentSlug = "icpnl"
	} else {
		parentSlug = "tronco"
	}

	url := fmt.Sprintf("https://gen.boletindiario.in/wp-json/wp/v2/categories?slug=%s", parentSlug)
	resp, _ := http.Get(url)
	if resp.StatusCode != 200 {
		logger.Error("Failed to get parent category ID: %s", resp.Status)
		panic(fmt.Sprintf("Failed to get category ID: %s", resp.Status))
	}
	defer resp.Body.Close()

	var parentCategories []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&parentCategories)

	if len(parentCategories) == 0 {
		logger.Warning("Parent category '%s' not found", parentSlug)
		return 0
	}

	parentID := int(parentCategories[0]["id"].(float64))

	childURL := fmt.Sprintf("https://gen.boletindiario.in/wp-json/wp/v2/categories?parent=%d&slug=%s", parentID, childSlug)
	resp2, _ := http.Get(childURL)
	if resp2.StatusCode != 200 {
		logger.Error("Failed to get child category ID: %s", resp2.Status)
		panic(fmt.Sprintf("Failed to get category ID: %s", resp2.Status))
	}
	defer resp2.Body.Close()

	var childCategories []map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&childCategories)

	if len(childCategories) == 0 {
		logger.Warning("Child category '%s' not found under parent '%s'", childSlug, parentSlug)
		return 0
	}
	return int(childCategories[0]["id"].(float64))
}

// sniffImageFromMagicBytes detects the real image format from file headers. CDNs often
// send misleading Content-Type, and URLs may say .jpg while bytes are WebP, AVIF, etc.
// Your WordPress can allow WebP/AVIF; REST upload still rejects when the declared
// Content-Type/filename does not match the real file bytes.
func sniffImageFromMagicBytes(b []byte) (contentType string, ext string, ok bool) {
	if len(b) < 12 {
		return "", "", false
	}
	switch {
	case b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "image/jpeg", ".jpg", true
	case len(b) >= 8 && string(b[0:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png", ".png", true
	case len(b) >= 6 && (string(b[0:6]) == "GIF87a" || string(b[0:6]) == "GIF89a"):
		return "image/gif", ".gif", true
	case string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp", ".webp", true
	case string(b[4:8]) == "ftyp":
		brand := string(b[8:12])
		if brand == "avif" || brand == "avis" {
			return "image/avif", ".avif", true
		}
		if len(b) >= 16 {
			boxSize := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
			if boxSize >= 16 && int(boxSize) <= len(b) {
				inner := b[12:boxSize]
				if bytes.Contains(inner, []byte("avif")) || bytes.Contains(inner, []byte("avis")) {
					return "image/avif", ".avif", true
				}
			}
		}
		return "", "", false
	default:
		return "", "", false
	}
}

func mimeFromContentTypeHeader(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	if i := strings.Index(h, ";"); i >= 0 {
		h = strings.TrimSpace(h[:i])
	}
	return h
}

func imageURLPathForSuffix(raw string) string {
	if i := strings.Index(raw, "?"); i >= 0 {
		raw = raw[:i]
	}
	return strings.ToLower(raw)
}

func guessImageFromURLPath(path string) (contentType string, ok bool) {
	switch {
	case strings.HasSuffix(path, ".avif"):
		return "image/avif", true
	case strings.HasSuffix(path, ".webp"):
		return "image/webp", true
	case strings.HasSuffix(path, ".png"):
		return "image/png", true
	case strings.HasSuffix(path, ".gif"):
		return "image/gif", true
	case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"), strings.HasSuffix(path, ".jpe"):
		return "image/jpeg", true
	default:
		return "", false
	}
}

func uploadFeaturedImage(imageURL string, postIndex int, token string) (int, error) {
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create image request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	var resp *http.Response
	err = withHTTPRetry(func() error {
		var doErr error
		resp, doErr = wordPressHTTPClient().Do(req)
		if doErr != nil {
			return doErr
		}
		if resp.StatusCode != 200 {
			if isRetryableHTTPStatus(resp.StatusCode) {
				resp.Body.Close()
				return markRetryable(fmt.Errorf("fetch image: HTTP %d", resp.StatusCode))
			}
			return fmt.Errorf("fetch image: HTTP %d", resp.StatusCode)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read image body: %w", err)
	}

	headerCT := mimeFromContentTypeHeader(resp.Header.Get("Content-Type"))
	sniffCT, sniffExt, sniffed := sniffImageFromMagicBytes(body)

	var contentType string
	var ext string
	if sniffed {
		contentType, ext = sniffCT, sniffExt
		if headerCT != "" && !strings.EqualFold(headerCT, sniffCT) {
			logger.Debug("Image: declared Content-Type %q differs from file signature %q (%s)", headerCT, sniffCT, imageURL)
		}
	} else {
		contentType = headerCT
		if !strings.HasPrefix(contentType, "image/") {
			if g, ok := guessImageFromURLPath(imageURLPathForSuffix(imageURL)); ok {
				contentType = g
				logger.Debug("Image: using type %q from URL path (%s)", contentType, imageURL)
			}
		}
		if !strings.HasPrefix(contentType, "image/") {
			return 0, fmt.Errorf("URL is not an image (Content-Type %q); if the URL is valid, the host may need different headers", headerCT)
		}
		exts, _ := mime.ExtensionsByType(contentType)
		ext = ".jpg"
		if len(exts) > 0 {
			ext = exts[0]
		}
	}

	fileName := generateRandomFilename() + ext
	url := "https://gen.boletindiario.in/wp-json/wp/v2/media"

	var mediaID int
	err = withHTTPRetry(func() error {
		uploadReq, reqErr := http.NewRequest("POST", url, bytes.NewReader(body))
		if reqErr != nil {
			return reqErr
		}
		uploadReq.Header.Add("Authorization", "Bearer "+token)
		uploadReq.Header.Add("Content-Type", contentType)
		uploadReq.Header.Add("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))

		res, doErr := wordPressHTTPClient().Do(uploadReq)
		if doErr != nil {
			return doErr
		}
		defer res.Body.Close()

		respBytes, readErr := io.ReadAll(res.Body)
		if readErr != nil {
			return readErr
		}

		if res.StatusCode != 201 {
			if isRetryableHTTPStatus(res.StatusCode) {
				return markRetryable(fmt.Errorf("upload to media: HTTP %d", res.StatusCode))
			}
			msg := strings.TrimSpace(string(respBytes))
			if len(msg) > 800 {
				msg = msg[:800] + "…"
			}
			var wrap struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			}
			if json.Unmarshal(respBytes, &wrap) == nil && wrap.Message != "" {
				detail := wrap.Message
				if wrap.Code != "" {
					detail = wrap.Code + ": " + detail
				}
				return fmt.Errorf("upload to media: HTTP %d — %s", res.StatusCode, detail)
			}
			if msg != "" {
				return fmt.Errorf("upload to media: HTTP %d — %s", res.StatusCode, msg)
			}
			return fmt.Errorf("upload to media: HTTP %d (empty body)", res.StatusCode)
		}

		var uploaded map[string]interface{}
		if jsonErr := json.Unmarshal(respBytes, &uploaded); jsonErr != nil {
			return fmt.Errorf("decode media response: %w", jsonErr)
		}
		id, ok := uploaded["id"].(float64)
		if !ok {
			return fmt.Errorf("media response missing id")
		}
		mediaID = int(id)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("upload to media: %w", err)
	}

	return mediaID, nil
}

func createPost(title, content string, categoryID, imageID, postIndex int, token string) error {
	postData := map[string]interface{}{
		"title":          title,
		"content":        content,
		"categories":     []int{categoryID},
		"tags":           []int{42, 40}, // index, icpnl (coparmex/tillit/grupo-senda deprecated)
		"featured_media": imageID,
		"status":         "publish",
	}

	jsonData, err := json.Marshal(postData)
	if err != nil {
		return fmt.Errorf("marshal post: %w", err)
	}

	postURL := "https://gen.boletindiario.in/wp-json/wp/v2/posts"
	err = withHTTPRetry(func() error {
		req, reqErr := http.NewRequest("POST", postURL, bytes.NewReader(jsonData))
		if reqErr != nil {
			return fmt.Errorf("create request: %w", reqErr)
		}
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Add("Content-Type", "application/json")

		resp, doErr := wordPressHTTPClient().Do(req)
		if doErr != nil {
			return doErr
		}
		defer resp.Body.Close()

		if resp.StatusCode == 201 {
			return nil
		}
		body, _ := io.ReadAll(resp.Body)
		logger.Debug("Response body: %s", string(body))
		if isRetryableHTTPStatus(resp.StatusCode) {
			return markRetryable(fmt.Errorf("WordPress returned HTTP %d", resp.StatusCode))
		}
		return fmt.Errorf("WordPress returned HTTP %d", resp.StatusCode)
	})
	if err != nil {
		return fmt.Errorf("post request: %w", err)
	}
	return nil
}

func generateRandomFilename() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, 4)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}
