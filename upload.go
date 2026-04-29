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
	payload := strings.NewReader(fmt.Sprintf("username=%s&password=%s", os.Getenv("EMAIL"), os.Getenv("PASSWORD")))
	req, err := http.NewRequest("POST", "https://gen.boletindiario.in/wp-json/jwt-auth/v1/token", payload)
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
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

func uploadFeaturedImage(imageURL string, postIndex int, token string) (int, error) {
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create image request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("fetch image: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read image body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	if contentType == "" {
		if strings.HasSuffix(strings.ToLower(imageURL), ".webp") {
			contentType = "image/webp"
		} else if strings.HasSuffix(strings.ToLower(imageURL), ".jpg") || strings.HasSuffix(strings.ToLower(imageURL), ".jpeg") {
			contentType = "image/jpeg"
		} else if strings.HasSuffix(strings.ToLower(imageURL), ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(strings.ToLower(imageURL), ".gif") {
			contentType = "image/gif"
		} else {
			contentType = "image/jpeg"
		}
		logger.Debug("Content-type detected from URL: %s", contentType)
	}

	if !strings.HasPrefix(contentType, "image/") {
		return 0, fmt.Errorf("URL is not an image (content-type: %s)", contentType)
	}

	exts, _ := mime.ExtensionsByType(contentType)
	ext := ".jpg"
	if len(exts) > 0 {
		ext = exts[0]
	}

	fileName := generateRandomFilename() + ext
	url := "https://gen.boletindiario.in/wp-json/wp/v2/media"

	uploadReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create upload request: %w", err)
	}
	uploadReq.Header.Add("Authorization", "Bearer "+token)
	uploadReq.Header.Add("Content-Type", contentType)
	uploadReq.Header.Add("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))

	res, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		return 0, fmt.Errorf("upload to media: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 201 {
		return 0, fmt.Errorf("upload to media: HTTP %d", res.StatusCode)
	}

	var uploaded map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&uploaded); err != nil {
		return 0, fmt.Errorf("decode media response: %w", err)
	}
	id, ok := uploaded["id"].(float64)
	if !ok {
		return 0, fmt.Errorf("media response missing id")
	}

	return int(id), nil
}

func createPost(title, content string, categoryID, imageID, postIndex int, token string) error {
	postData := map[string]interface{}{
		"title":          title,
		"content":        content,
		"categories":     []int{categoryID},
		"tags":           []int{34, 35, 36},
		"featured_media": imageID,
		"status":         "publish",
	}

	if categoryID == 30 || categoryID == 31 {
		postData["tags"] = append(postData["tags"].([]int), 46)
	}
	if categoryID == 28 || categoryID == 31 || categoryID == 33 {
		postData["tags"] = append(postData["tags"].([]int), 52)
	}

	jsonData, err := json.Marshal(postData)
	if err != nil {
		return fmt.Errorf("marshal post: %w", err)
	}

	req, err := http.NewRequest("POST", "https://gen.boletindiario.in/wp-json/wp/v2/posts", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		logger.Debug("Response body: %s", string(body))
		return fmt.Errorf("WordPress returned HTTP %d", resp.StatusCode)
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
