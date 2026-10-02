"""Read-only production policy checks and a non-mutating admin dry-run."""
import http.cookiejar
import json
from pathlib import Path
import shlex
import urllib.error
import urllib.request

base = "https://admin.hsgram.cloud"
for path in ("/api/server/login-policy", "/api/server/registration-invites"):
    try:
        urllib.request.urlopen(base + path, timeout=20)
        raise AssertionError("Unauthenticated policy read accepted")
    except urllib.error.HTTPError as error:
        assert error.code == 401, error.code
env = {}
for line in Path("/home/safelink-chat/admin.env").read_text().splitlines():
    if "=" in line and not line.lstrip().startswith("#"):
        key, value = line.split("=", 1)
        values = shlex.split(value)
        env[key] = values[0] if values else ""
jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
def request(path, body=None, csrf=None):
    headers = {"Content-Type": "application/json", "Origin": base}
    if csrf:
        headers["X-CSRF-Token"] = csrf
    data = None if body is None else json.dumps(body).encode()
    with opener.open(urllib.request.Request(base + path, data=data, headers=headers), timeout=30) as response:
        raw = response.read()
        return json.loads(raw) if raw else None
login = request("/api/login", {"username": "admin", "secret": env["TELESRV_ADMIN_UI_PASSWORD"]})
csrf = login["csrf_token"]
assert all(cookie.secure for cookie in jar)
try:
    policy = request("/api/server/login-policy")
    assert isinstance(policy["future_auth_enabled"], bool)
    assert isinstance(policy["registration_password_required"], bool)
    assert 1 <= policy["future_auth_days"] <= 365
    print("PASS: authenticated login policy", json.dumps(policy))
    invites = request("/api/server/registration-invites")
    assert isinstance(invites["enabled"], bool) and isinstance(invites["items"], list)
    print("PASS: invitation policy; enabled=", invites["enabled"])
    dry = request("/api/actions/login-policy", dict(policy, reason="deployment validation", confirm=False), csrf)
    assert dry["dry_run"] is True and not dry.get("error"), dry
    assert request("/api/server/login-policy") == policy
    print("PASS: dry-run leaves production policy unchanged")
finally:
    request("/api/logout", {}, csrf)
print("PASS: HTTPS admin login, secure cookies, protected policy APIs and logout")
