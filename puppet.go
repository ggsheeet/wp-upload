package main

// import (
// 	"context"
// 	"encoding/base64"
// 	"encoding/json"
// 	"flag"
// 	"io"
// 	"net/http"
// 	"os"
// 	"path/filepath"
// 	"strings"
// 	"sync"
// 	"time"

// 	"github.com/chromedp/cdproto/network"
// 	"github.com/chromedp/chromedp"
// )

// var logger = NewColoredLogger("", nil)

// type ConsultaResponse struct {
// 	Registros []struct {
// 		Parametros string `json:"parametros"`
// 	} `json:"registros"`
// }

// type ResponseCapture struct {
// 	params   string
// 	captured bool
// 	mutex    sync.RWMutex
// }

// func (rc *ResponseCapture) Set(params string) {
// 	rc.mutex.Lock()
// 	defer rc.mutex.Unlock()
// 	if !rc.captured {
// 		rc.params = params
// 		rc.captured = true
// 	}
// }

// func (rc *ResponseCapture) Get() (string, bool) {
// 	rc.mutex.RLock()
// 	defer rc.mutex.RUnlock()
// 	return rc.params, rc.captured
// }

// func main() {
// 	// CLI flags
// 	logger.Info("🚀 Starting CURP automation program...")
// 	curp := flag.String("curp", "", "CURP value to search (required)")
// 	downloadDir := flag.String("out", "./curp-pdf", "Directory for downloads")
// 	headless := flag.Bool("headless", true, "Run Chrome headless")
// 	timeout := flag.Duration("timeout", 5*time.Minute, "Overall timeout")
// 	flag.Parse()

// 	logger.Info("📋 Configuration: curp=%s, out=%s, headless=%t, timeout=%s", *curp, *downloadDir, *headless, *timeout)

// 	if *curp == "" {
// 		logger.Error("❌ Missing -curp value")
// 		os.Exit(2)
// 	}

// 	// Create output directory
// 	if err := os.MkdirAll(*downloadDir, 0755); err != nil {
// 		logger.Error("❌ Failed to create directory %s: %v", *downloadDir, err)
// 		os.Exit(1)
// 	}
// 	logger.Info("📁 Output directory ready: %s", *downloadDir)

// 	// Setup Chrome context with proper cleanup
// 	ctx, cancel := setupBrowserContext(*headless, *timeout)
// 	defer cancel()

// 	// Enable network monitoring
// 	if err := chromedp.Run(ctx, network.Enable()); err != nil {
// 		logger.Error("❌ Failed to enable network monitoring: %v", err)
// 		os.Exit(1)
// 	}

// 	// Setup response capture with thread safety
// 	responseCapture := &ResponseCapture{}
// 	setupNetworkListener(ctx, responseCapture)

// 	// Execute automation steps
// 	tables := executeAutomation(ctx, *curp)

// 	// Save table data
// 	saveTableData(tables, *downloadDir)

// 	// Download PDF using captured parameters or fallback to button click
// 	downloadPDF(ctx, responseCapture, *curp, *downloadDir)

// 	logger.Info("🎉 CURP automation completed successfully!")
// }

// func setupBrowserContext(headless bool, timeout time.Duration) (context.Context, context.CancelFunc) {
// 	logger.Info("🌐 Setting up browser context...")

// 	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
// 		append(chromedp.DefaultExecAllocatorOptions[:],
// 			chromedp.Flag("headless", headless),
// 			chromedp.Flag("disable-gpu", true),
// 			chromedp.Flag("no-sandbox", true),
// 			chromedp.Flag("disable-dev-shm-usage", true),
// 			chromedp.Flag("disable-web-security", true),
// 			chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"),
// 		)...,
// 	)

// 	ctx, cancel := chromedp.NewContext(allocCtx)
// 	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)

// 	// Return combined cancel function
// 	return ctx, func() {
// 		cancelTimeout()
// 		cancel()
// 		cancelAlloc()
// 	}
// }

// func setupNetworkListener(ctx context.Context, responseCapture *ResponseCapture) {
// 	logger.Info("🕸️ Setting up network event listener...")

// 	chromedp.ListenTarget(ctx, func(ev interface{}) {
// 		switch e := ev.(type) {
// 		case *network.EventResponseReceived:
// 			if strings.Contains(e.Response.URL, "/v1/renapoCURP/consulta") {
// 				logger.Info("🎯 Detected consulta response!")

// 				// Use a separate goroutine with timeout to avoid blocking
// 				go func() {
// 					// Create a timeout context for this operation
// 					captureCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 					defer cancel()

// 					body, err := network.GetResponseBody(e.RequestID).Do(captureCtx)
// 					if err != nil {
// 						logger.Error("⚠️ Failed to capture response body: %v", err)
// 						return
// 					}

// 					var response ConsultaResponse
// 					if err := json.Unmarshal(body, &response); err != nil {
// 						logger.Error("⚠️ Failed to parse JSON response: %v", err)
// 						return
// 					}

// 					if len(response.Registros) > 0 && response.Registros[0].Parametros != "" {
// 						responseCapture.Set(response.Registros[0].Parametros)
// 						logger.Info("✅ Successfully captured PDF parameters!")
// 					} else {
// 						logger.Error("⚠️ No valid parameters found in response")
// 					}
// 				}()
// 			}
// 		}
// 	})
// }

// func executeAutomation(ctx context.Context, curp string) []map[string]string {
// 	logger.Info("🤖 Starting browser automation...")

// 	// Navigate to website
// 	if err := chromedp.Run(ctx, chromedp.Navigate("https://www.gob.mx/curp/")); err != nil {
// 		logger.Error("❌ Failed to navigate to website: %v", err)
// 		os.Exit(1)
// 	}
// 	logger.Info("🌍 Navigated to CURP website")

// 	// Fill and submit form
// 	if err := chromedp.Run(ctx,
// 		chromedp.WaitVisible("#curpinput", chromedp.ByID),
// 		chromedp.Clear("#curpinput", chromedp.ByID),
// 		chromedp.SendKeys("#curpinput", curp, chromedp.ByID),
// 		chromedp.Click("#searchButton", chromedp.ByID),
// 	); err != nil {
// 		logger.Error("❌ Failed to submit form: %v", err)
// 		os.Exit(1)
// 	}
// 	logger.Info("📝 Form submitted successfully")

// 	// Wait for results with proper timeout and debugging
// 	logger.Info("⏳ Waiting for Ember.js to render results...")
// 	if err := chromedp.Run(ctx,
// 		chromedp.Sleep(5*time.Second), // Longer initial wait for API call
// 		chromedp.ActionFunc(func(ctx context.Context) error {
// 			logger.Info("🔍 Checking if tables exist in DOM...")
// 			return nil
// 		}),
// 		chromedp.WaitVisible("table", chromedp.ByQuery),
// 	); err != nil {
// 		logger.Info("⚠️ Direct table wait failed, trying longer wait...")
// 		// Fallback: wait longer and check what's actually on the page
// 		if err := chromedp.Run(ctx,
// 			chromedp.Sleep(10*time.Second), // Much longer wait
// 			chromedp.ActionFunc(func(ctx context.Context) error {
// 				logger.Info("🔍 Checking page content after extended wait...")
// 				return nil
// 			}),
// 		); err != nil {
// 			logger.Error("❌ Extended wait failed: %v", err)
// 			os.Exit(1)
// 		}
// 	}
// 	logger.Info("✅ Ember.js rendering wait completed")

// 	// Extract table data
// 	var tables []map[string]string
// 	if err := chromedp.Run(ctx,
// 		chromedp.Sleep(2*time.Second), // Ensure full rendering
// 		chromedp.Evaluate(`(() => {
// 			const tables = Array.from(document.querySelectorAll('table'));
// 			return tables.map(table => {
// 				const data = {};
// 				const rows = table.querySelectorAll('tr');
// 				rows.forEach(row => {
// 					const cells = row.querySelectorAll('td');
// 					if (cells.length >= 2) {
// 						const key = cells[0].innerText.trim().replace(/:\s*$/, '');
// 						const value = cells[1].innerText.trim();
// 						if (key && value) {
// 							data[key] = value;
// 						}
// 					}
// 				});
// 				return data;
// 			}).filter(table => Object.keys(table).length > 0);
// 		})()`, &tables),
// 	); err != nil {
// 		logger.Error("❌ Failed to extract table data: %v", err)
// 		os.Exit(1)
// 	}

// 	logger.Info("✅ Extracted data from %d tables", len(tables))
// 	return tables
// }

// func saveTableData(tables []map[string]string, downloadDir string) {
// 	logger.Info("💾 Saving table data...")

// 	tablePath := filepath.Join(downloadDir, "tables.json")
// 	data, err := json.MarshalIndent(tables, "", "  ")
// 	if err != nil {
// 		logger.Error("❌ Failed to marshal table data: %v", err)
// 		return
// 	}

// 	if err := os.WriteFile(tablePath, data, 0644); err != nil {
// 		logger.Error("❌ Failed to save table data: %v", err)
// 		return
// 	}

// 	logger.Info("✅ Table data saved to %s", tablePath)
// 	for i, table := range tables {
// 		logger.Info("   📋 Table %d: %d entries", i+1, len(table))
// 	}
// }

// func downloadPDF(ctx context.Context, responseCapture *ResponseCapture, curp, downloadDir string) {
// 	logger.Info("📄 Attempting to download PDF...")

// 	// Wait for response capture with timeout
// 	params, captured := waitForParameters(responseCapture, 15*time.Second)

// 	if captured && params != "" {
// 		// Method 1: Direct HTTP download using captured parameters
// 		if downloadPDFDirect(params, curp, downloadDir) {
// 			return
// 		}
// 		logger.Info("🔄 Direct download failed, trying download button...")
// 	}

// 	// Method 2: Fallback to clicking download button
// 	downloadPDFViaButton(ctx, curp, downloadDir)
// }

// func waitForParameters(responseCapture *ResponseCapture, timeout time.Duration) (string, bool) {
// 	logger.Info("⏳ Waiting for PDF parameters...")

// 	deadline := time.Now().Add(timeout)
// 	for time.Now().Before(deadline) {
// 		if params, captured := responseCapture.Get(); captured {
// 			logger.Info("✅ Parameters received: %s", params[:min(50, len(params))]+"...")
// 			return params, true
// 		}
// 		time.Sleep(500 * time.Millisecond)
// 	}

// 	logger.Info("⏰ Timeout waiting for parameters")
// 	return "", false
// }

// func downloadPDFDirect(params, curp, downloadDir string) bool {
// 	logger.Info("🌐 Downloading PDF directly...")

// 	url := "https://consultas.curp.gob.mx/CurpSP/pdfgobmx" + params

// 	client := &http.Client{Timeout: 30 * time.Second}
// 	resp, err := client.Get(url)
// 	if err != nil {
// 		logger.Error("⚠️ HTTP request failed: %v", err)
// 		return false
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != 200 {
// 		logger.Error("⚠️ Unexpected status code: %d", resp.StatusCode)
// 		return false
// 	}

// 	body, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		logger.Error("⚠️ Failed to read response: %v", err)
// 		return false
// 	}

// 	pdfData, err := base64.StdEncoding.DecodeString(string(body))
// 	if err != nil {
// 		logger.Error("⚠️ Failed to decode base64: %v", err)
// 		return false
// 	}

// 	pdfPath := filepath.Join(downloadDir, curp+".pdf")
// 	if err := os.WriteFile(pdfPath, pdfData, 0644); err != nil {
// 		logger.Error("⚠️ Failed to save PDF: %v", err)
// 		return false
// 	}

// 	logger.Info("✅ PDF downloaded successfully: %s (%d bytes)", pdfPath, len(pdfData))
// 	return true
// }

// func downloadPDFViaButton(ctx context.Context, curp, downloadDir string) {
// 	logger.Info("🖱️ Attempting download via button click...")

// 	// Wait for download button to be visible (it's already enabled)
// 	if err := chromedp.Run(ctx,
// 		chromedp.WaitVisible("#download", chromedp.ByID),
// 		chromedp.ActionFunc(func(ctx context.Context) error {
// 			logger.Info("✅ Download button found")
// 			return nil
// 		}),
// 	); err != nil {
// 		logger.Error("❌ Download button not found: %v", err)
// 		return
// 	}

// 	// Click the download button
// 	if err := chromedp.Run(ctx,
// 		chromedp.Click("#download", chromedp.ByID),
// 		chromedp.Sleep(3*time.Second), // Wait for download to start
// 	); err != nil {
// 		logger.Error("❌ Failed to click download button: %v", err)
// 		return
// 	}

// 	logger.Info("✅ Download button clicked successfully")
// 	logger.Info("ℹ️ PDF should be downloaded to your default downloads folder")
// 	logger.Info("ℹ️ You may need to manually move it to: %s", downloadDir)
// }

// func min(a, b int) int {
// 	if a < b {
// 		return a
// 	}
// 	return b
// }
