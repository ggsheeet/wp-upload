package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func formatDoc() {
	inputFile := "posts.txt"
	content, err := readLines(inputFile)
	if err != nil {
		logger.Error("Error reading file: %v", err)
		return
	}

	formattedPosts := formatMultiplePosts(content)
	finalContent := strings.Join(formattedPosts, "\n\n")

	err = os.WriteFile(inputFile, []byte(finalContent), 0644)
	if err != nil {
		logger.Error("Error writing file: %v", err)
		return
	}

	logger.Info("File formatted and overwritten successfully.")
}

func readLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func isURL(text string) bool {
	return strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://")
}

func normalizeCategory(rawCategory string) string {
	categoryMap := map[string]string{
		"FISCAL":                 "fiscal",
		"LABORAL":                "laboral",
		"COMERCIO EXTERIOR":      "comercio-exterior",
		"NACIONALES":             "nacional",
		"EMPRESAS":               "empresas",
		"FINANZAS":               "finanzas",
		"RESPONSABILIDAD SOCIAL": "rse",
	}

	normalized := strings.ToUpper(strings.TrimSpace(rawCategory))
	if slug, exists := categoryMap[normalized]; exists {
		return slug
	}
	return ""
}

type RawPost struct {
	Category string
	Title    string
	Lines    []string
}

func analyzePostStructure(lines []string) (content, newspaper, url string) {
	if len(lines) == 0 {
		return "", "", ""
	}

	var urlIndex, newspaperIndex int = -1, -1

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		if urlIndex == -1 && isURL(line) {
			urlIndex = i
			url = line
		}
	}

	if urlIndex > 0 {
		newspaperIndex = urlIndex - 1
		newspaper = strings.TrimSpace(lines[newspaperIndex])
	}

	var contentLines []string
	endIndex := len(lines)
	if newspaperIndex != -1 {
		endIndex = newspaperIndex
	} else if urlIndex != -1 {
		endIndex = urlIndex
	}

	for i := 0; i < endIndex; i++ {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			contentLines = append(contentLines, line)
		}
	}

	content = strings.Join(contentLines, " ")
	return content, newspaper, url
}

func splitPostsByStructure(lines []string, category string) []RawPost {
	var posts []RawPost
	var currentLines []string
	var title string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if title != "" && isURL(line) {
			currentLines = append(currentLines, line)

			content, newspaper, url := analyzePostStructure(currentLines)
			posts = append(posts, RawPost{
				Category: category,
				Title:    title,
				Lines:    []string{content, newspaper, url},
			})

			title = ""
			currentLines = []string{}
		} else if title == "" && !isURL(line) {
			title = line
		} else {
			currentLines = append(currentLines, line)
		}
	}

	if title != "" {
		content, newspaper, url := analyzePostStructure(currentLines)
		posts = append(posts, RawPost{
			Category: category,
			Title:    title,
			Lines:    []string{content, newspaper, url},
		})
	}

	return posts
}

func formatMultiplePosts(content []string) []string {
	validCategories := map[string]bool{
		"fiscal": true, "laboral": true, "comercio-exterior": true,
		"nacional": true, "empresas": true, "finanzas": true, "rse": true,
	}

	var allPosts []RawPost
	var currentCategory string
	var categoryLines []string
	var foundFirstCategory bool

	for _, rawLine := range content {
		line := strings.TrimSpace(rawLine)

		if line == "" {
			continue
		}

		normalizedCategory := normalizeCategory(line)
		if normalizedCategory != "" && validCategories[normalizedCategory] {
			foundFirstCategory = true
			if currentCategory != "" {
				posts := splitPostsByStructure(categoryLines, currentCategory)
				allPosts = append(allPosts, posts...)
			}
			currentCategory = normalizedCategory
			categoryLines = []string{}
		} else if foundFirstCategory && currentCategory != "" {
			categoryLines = append(categoryLines, line)
		}
	}

	if currentCategory != "" {
		posts := splitPostsByStructure(categoryLines, currentCategory)
		allPosts = append(allPosts, posts...)
	}

	var formattedPosts []string
	for _, rawPost := range allPosts {
		var structuredContent string
		for _, line := range rawPost.Lines {
			if line != "" {
				if structuredContent != "" {
					structuredContent += "\n" + line
				} else {
					structuredContent = line
				}
			}
		}

		formattedPosts = append(formattedPosts, formatPost(rawPost.Category, rawPost.Title, structuredContent))
	}

	return formattedPosts
}

func formatPost(category, title, content string) string {
	return fmt.Sprintf("Title: %s\nCategory: %s\nImage: \n%s", title, category, strings.TrimSpace(content))
}
