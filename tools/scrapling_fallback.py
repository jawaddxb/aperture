#!/usr/bin/env python3
"""Scrapling fallback fetcher for Aperture.

Called as a subprocess when Aperture detects a Cloudflare challenge.
Takes a URL, returns JSON with page HTML and/or a base64 screenshot.

Usage:
    python3 scrapling_fallback.py --url <url> [--screenshot] [--timeout 30]
"""
import argparse
import base64
import json
import sys
import tempfile
import os


def main():
    parser = argparse.ArgumentParser(description="Scrapling fallback fetcher")
    parser.add_argument("--url", required=True, help="URL to fetch")
    parser.add_argument("--screenshot", action="store_true", help="Take screenshot")
    parser.add_argument("--timeout", type=int, default=30, help="Timeout in seconds")
    args = parser.parse_args()

    try:
        from scrapling import StealthyFetcher

        screenshot_path = None
        if args.screenshot:
            screenshot_path = os.path.join(tempfile.gettempdir(), "scrapling_screenshot.png")

        def take_screenshot(page):
            """page_action callback — runs inside the stealth browser context."""
            if screenshot_path:
                page.screenshot(path=screenshot_path, full_page=False)

        page = StealthyFetcher.fetch(
            args.url,
            headless=True,
            solve_cloudflare=True,
            hide_canvas=True,
            block_webrtc=True,
            network_idle=True,
            timeout=args.timeout * 1000,
            page_action=take_screenshot if args.screenshot else None,
        )

        # Extract HTML
        html = ""
        if hasattr(page, "html_content"):
            html = page.html_content[:500_000]
        elif hasattr(page, "text"):
            html = page.text[:500_000]
        else:
            html = str(page)[:500_000]

        # Extract title
        title = ""
        try:
            if hasattr(page, "css_first"):
                el = page.css_first("title")
                title = el.text() if el else ""
        except Exception:
            pass

        # Read screenshot if taken
        screenshot_b64 = None
        if screenshot_path and os.path.exists(screenshot_path):
            with open(screenshot_path, "rb") as f:
                screenshot_b64 = base64.b64encode(f.read()).decode("utf-8")
            os.unlink(screenshot_path)

        result = {
            "ok": True,
            "html": html,
            "title": title,
            "url": args.url,
            "screenshot_b64": screenshot_b64,
        }
        json.dump(result, sys.stdout)

    except Exception as e:
        json.dump({"ok": False, "error": str(e)}, sys.stdout)
        sys.exit(1)


if __name__ == "__main__":
    main()
