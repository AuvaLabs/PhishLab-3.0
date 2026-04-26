#!/usr/bin/env python3
"""Capture documentation screenshots for PhishRig."""

import argparse
import glob
import json
import os
import sys
import time
import urllib.request

from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.chrome.options import Options
from selenium.webdriver.chrome.service import Service
from selenium.webdriver.support import expected_conditions as EC
from selenium.webdriver.support.ui import WebDriverWait

import chromedriver_autoinstaller


def create_driver():
    """Create headless Chromium driver."""
    chromedriver_autoinstaller.install()
    options = Options()
    options.binary_location = "/snap/bin/chromium"
    options.add_argument("--headless=new")
    options.add_argument("--no-sandbox")
    options.add_argument("--disable-dev-shm-usage")
    options.add_argument("--disable-gpu")
    options.add_argument("--window-size=1440,900")
    options.add_argument("--ignore-certificate-errors")
    options.add_argument("--allow-insecure-localhost")
    options.add_argument("--remote-debugging-port=9222")
    options.add_argument("--disable-software-rasterizer")
    options.add_argument(f"--user-data-dir=/tmp/chromium-screenshot-{os.getpid()}")
    driver = webdriver.Chrome(options=options)
    driver.set_window_size(1440, 900)
    return driver


def gophish_login(driver, base_url, username, password):
    """Log into Gophish and return the session."""
    driver.get(f"{base_url}/login")
    time.sleep(2)

    user_field = driver.find_element(By.NAME, "username")
    pass_field = driver.find_element(By.NAME, "password")
    user_field.clear()
    user_field.send_keys(username)
    pass_field.clear()
    pass_field.send_keys(password)
    # Submit the form via the submit button
    submit_btn = driver.find_element(By.CSS_SELECTOR, "button[type='submit']")
    submit_btn.click()
    time.sleep(4)
    # Check if login succeeded by looking at URL
    if "/login" in driver.current_url:
        print("  Warning: login may have failed, still on login page")
        # Try submitting the form via JS
        driver.execute_script("document.querySelector('form.form-signin').submit()")
        time.sleep(3)
    return driver


def get_gophish_api_key(driver, base_url):
    """Extract API key from Gophish settings page."""
    driver.get(f"{base_url}/settings")
    time.sleep(2)
    try:
        # Gophish shows API key in a readonly input or span
        api_input = driver.find_element(By.CSS_SELECTOR, "input[name='api_key'], #api_key, input.form-control[readonly], span.api-key")
        return api_input.get_attribute("value") or api_input.text
    except Exception:
        pass
    # Try to find it in the page source
    import re
    src = driver.page_source
    match = re.search(r'[a-f0-9]{32,}', src)
    if match:
        return match.group(0)
    return None


def gophish_api(base_url, api_key, endpoint, method="GET", data=None):
    """Make a Gophish API request."""
    url = f"{base_url}/api/{endpoint}?api_key={api_key}"
    req = urllib.request.Request(url, method=method)
    req.add_header("Content-Type", "application/json")
    if data:
        req.data = json.dumps(data).encode()
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read())
    except Exception as e:
        print(f"  API error ({endpoint}): {e}")
        return None


def setup_gophish_demo(base_url, api_key, target_email):
    """Create demo sending profile, template, group, and campaign in Gophish."""
    first, last = "Nicholai", "Imbong"
    email_parts = target_email.split("@")
    domain = email_parts[1] if len(email_parts) > 1 else "auvalabs.com"

    # 1. Create sending profile (Mailhog)
    profile = gophish_api(base_url, api_key, "smtp/", "POST", {
        "name": "PhishRig - Mailhog",
        "host": "localhost:1025",
        "from_address": f"IT Security <security@{domain}>",
        "ignore_cert_errors": True,
    })
    profile_id = profile.get("id") if profile else None
    print(f"  Sending profile: {'created' if profile_id else 'failed'}")

    # 2. Create email template
    template = gophish_api(base_url, api_key, "templates/", "POST", {
        "name": "O365 Password Expiry",
        "subject": "Action Required: Your password expires in 24 hours",
        "html": """<html><body style="font-family:Segoe UI,sans-serif;max-width:600px;margin:0 auto;padding:20px">
<div style="background:#0078d4;padding:20px;text-align:center">
<h2 style="color:white;margin:0">Microsoft 365</h2>
</div>
<div style="padding:20px;border:1px solid #ddd;border-top:none">
<p>Hello {{.FirstName}},</p>
<p>Your Microsoft 365 password will expire in <strong>24 hours</strong>.
To avoid losing access to your account, please update your password now.</p>
<p style="text-align:center;margin:30px 0">
<a href="{{.URL}}" style="background:#0078d4;color:white;padding:12px 30px;text-decoration:none;border-radius:4px;font-size:16px">Update Password</a>
</p>
<p style="color:#666;font-size:12px">If you did not request this, please contact IT support immediately.</p>
<hr style="border:none;border-top:1px solid #eee">
<p style="color:#999;font-size:11px">Microsoft Corporation, One Microsoft Way, Redmond, WA 98052</p>
</div></body></html>""",
        "text": "Hello {{.FirstName}}, your password expires in 24 hours. Update: {{.URL}}",
    })
    template_id = template.get("id") if template else None
    print(f"  Email template: {'created' if template_id else 'failed'}")

    # 3. Create target group
    group = gophish_api(base_url, api_key, "groups/", "POST", {
        "name": "O365 Targets",
        "targets": [
            {"first_name": first, "last_name": last, "email": target_email, "position": "Security Engineer"},
        ],
    })
    group_id = group.get("id") if group else None
    print(f"  Target group: {'created' if group_id else 'failed'}")

    # 4. Create landing page (placeholder — Evilginx handles actual landing)
    page = gophish_api(base_url, api_key, "pages/", "POST", {
        "name": "O365 Login",
        "html": "<html><body><h1>Redirecting...</h1></body></html>",
        "capture_credentials": False,
        "redirect_url": "https://login.microsoftonline.com",
    })
    page_id = page.get("id") if page else None
    print(f"  Landing page: {'created' if page_id else 'failed'}")

    return {
        "profile_id": profile_id,
        "template_id": template_id,
        "group_id": group_id,
        "page_id": page_id,
    }


def capture_screenshots(driver, args, output_dir):
    """Capture all documentation screenshots."""
    os.makedirs(output_dir, exist_ok=True)
    screenshots = []

    # 1. Gophish Login Page
    print("[1/7] Gophish login page...")
    driver.get(f"{args.gophish_url}/login")
    time.sleep(2)
    path = os.path.join(output_dir, "01-gophish-login.png")
    driver.save_screenshot(path)
    screenshots.append(path)
    print(f"  Saved: {path}")

    # 2. Login and capture dashboard
    print("[2/7] Gophish dashboard...")
    gophish_login(driver, args.gophish_url, args.gophish_user, args.gophish_pass)
    time.sleep(2)
    path = os.path.join(output_dir, "02-gophish-dashboard.png")
    driver.save_screenshot(path)
    screenshots.append(path)
    print(f"  Saved: {path}")

    # Get API key
    api_key = get_gophish_api_key(driver, args.gophish_url)
    if api_key:
        print(f"  API key found: {api_key[:8]}...")
    else:
        print("  Warning: could not extract API key, skipping demo setup")

    # 3. Set up demo data and capture template
    if api_key:
        print("[3/7] Setting up demo campaign data...")
        setup_gophish_demo(args.gophish_url, api_key, args.target_email)
        time.sleep(1)

        driver.get(f"{args.gophish_url}/templates")
        time.sleep(2)
        path = os.path.join(output_dir, "03-email-template.png")
        driver.save_screenshot(path)
        screenshots.append(path)
        print(f"  Saved: {path}")
    else:
        print("[3/7] Skipping template screenshot (no API key)")

    # 4. Campaign page
    print("[4/7] Campaign page...")
    driver.get(f"{args.gophish_url}/campaigns")
    time.sleep(2)
    path = os.path.join(output_dir, "04-campaigns.png")
    driver.save_screenshot(path)
    screenshots.append(path)
    print(f"  Saved: {path}")

    # 5. Mailhog inbox
    print("[5/7] Mailhog inbox...")
    driver.get(args.mailhog_url)
    time.sleep(2)
    path = os.path.join(output_dir, "05-mailhog-inbox.png")
    driver.save_screenshot(path)
    screenshots.append(path)
    print(f"  Saved: {path}")

    # 6. Phishing page (Evilginx proxied login)
    if args.phishing_url:
        print("[6/7] Phishing login page...")
        try:
            driver.get(args.phishing_url)
            time.sleep(8)  # Wait for Evilginx proxy + page render
            path = os.path.join(output_dir, "06-phishing-page.png")
            driver.save_screenshot(path)
            screenshots.append(path)
            print(f"  Saved: {path}")
        except Exception as e:
            print(f"  Failed: {e}")
    else:
        print("[6/7] Skipping phishing page (no URL provided)")

    # 7. PhishRig dashboard
    print("[7/7] PhishRig dashboard...")
    driver.get(args.dashboard_url)
    time.sleep(3)
    path = os.path.join(output_dir, "07-phishrig-dashboard.png")
    driver.save_screenshot(path)
    screenshots.append(path)
    print(f"  Saved: {path}")

    return screenshots, api_key


def create_gif(screenshots, output_path):
    """Create animated GIF from screenshots."""
    from PIL import Image

    if len(screenshots) < 2:
        print("Not enough screenshots for GIF")
        return

    images = []
    for path in screenshots:
        img = Image.open(path)
        img = img.resize((1200, 750), Image.LANCZOS)
        # Convert to palette mode for smaller GIF
        img = img.convert("RGB").quantize(colors=256)
        images.append(img)

    images[0].save(
        output_path,
        save_all=True,
        append_images=images[1:],
        duration=3000,
        loop=0,
        optimize=True,
    )
    size_mb = os.path.getsize(output_path) / (1024 * 1024)
    print(f"  GIF saved: {output_path} ({size_mb:.1f} MB)")


def main():
    parser = argparse.ArgumentParser(description="Capture PhishRig documentation screenshots")
    parser.add_argument("--gophish-url", default="http://127.0.0.1:8800")
    parser.add_argument("--gophish-user", default="admin")
    parser.add_argument("--gophish-pass", required=True)
    parser.add_argument("--mailhog-url", default="http://127.0.0.1:8025")
    parser.add_argument("--dashboard-url", default="http://127.0.0.1:8443")
    parser.add_argument("--phishing-url", default=None, help="Evilginx lure URL")
    parser.add_argument("--target-email", default="target@example.com")
    parser.add_argument("--output-dir", default="docs/images")
    parser.add_argument("--no-gif", action="store_true")
    args = parser.parse_args()

    print("=== PhishRig Documentation Screenshot Capture ===\n")

    driver = create_driver()
    try:
        screenshots, api_key = capture_screenshots(driver, args, args.output_dir)
        print(f"\nCaptured {len(screenshots)} screenshots")

        if not args.no_gif and len(screenshots) >= 2:
            print("\nGenerating pipeline GIF...")
            gif_path = os.path.join(args.output_dir, "phishrig-pipeline.gif")
            create_gif(screenshots, gif_path)

        if api_key:
            print(f"\nGophish API key: {api_key}")
            print("Add this to your phishrig.yaml: gophish.api_key")

    finally:
        driver.quit()

    print("\nDone!")


if __name__ == "__main__":
    main()
