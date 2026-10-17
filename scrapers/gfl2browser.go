package scrapers

import (
	"fmt"
	"log"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// gfl2RenderPage returns the HTML of a page after its JavaScript has run. It
// is a variable so tests can replace it; the default drives headless Chromium.
var gfl2RenderPage = renderPageWithBrowser

// renderPageWithBrowser opens url in headless Chromium, waits until at least
// one element matching selector exists and the list has stopped growing, and
// returns the rendered HTML. The Playwright driver and Chromium are downloaded
// the first time they are needed.
func renderPageWithBrowser(url, selector string) (string, error) {
	pw, err := playwright.Run()
	var browser playwright.Browser
	if err == nil {
		browser, err = launchChromium(pw)
	}
	if err != nil {
		// Probably not installed yet: install the driver and Chromium, once.
		if pw != nil {
			_ = pw.Stop()
		}
		log.Printf("[Playwright] starting Chromium failed (%v); installing Playwright and Chromium (first run only)...", err)
		if ierr := playwright.Install(&playwright.RunOptions{Browsers: []string{"chromium"}}); ierr != nil {
			return "", fmt.Errorf("install playwright: %w", ierr)
		}
		if pw, err = playwright.Run(); err != nil {
			return "", fmt.Errorf("start playwright: %w", err)
		}
		if browser, err = launchChromium(pw); err != nil {
			_ = pw.Stop()
			return "", fmt.Errorf("launch chromium: %w", err)
		}
	}
	defer pw.Stop()
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		return "", fmt.Errorf("new page: %w", err)
	}
	if _, err := page.Goto(url, playwright.PageGotoOptions{Timeout: playwright.Float(60000)}); err != nil {
		return "", fmt.Errorf("open %s: %w", url, err)
	}
	if _, err := page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{Timeout: playwright.Float(30000)}); err != nil {
		return "", fmt.Errorf("wait for %q: %w", selector, err)
	}
	// Let the rest of the list render: wait for the network to go quiet (best
	// effort), then until the number of matching elements stops changing.
	_ = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(10000),
	})
	loc := page.Locator(selector)
	last, stable := -1, 0
	for i := 0; i < 40 && stable < 3; i++ {
		n, err := loc.Count()
		if err == nil && n == last {
			stable++
		} else {
			stable, last = 0, n
		}
		time.Sleep(300 * time.Millisecond)
	}
	return page.Content()
}

func launchChromium(pw *playwright.Playwright) (playwright.Browser, error) {
	return pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
}
