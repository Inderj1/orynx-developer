// Orynx auto-login bridge.
//
// This box is a private, single-user, IP-locked instance (the security group
// restricts every port to the operator's /32). There is intentionally NO
// interactive login: a cookieless request is silently signed in as the single
// operator account and redirected to the dashboard.
//
// It mints a real Orynx session the sanctioned way — send-code + verify-code
// against the loopback backend — and relays the backend's own Set-Cookie
// headers (multica_auth JWT + multica_csrf) to the browser. No secrets or JWT
// signing logic are reimplemented here.
//
// Zero external dependencies (Node built-in http only) to keep the added attack
// surface minimal and auditable.

const http = require("http");

const BACKEND = process.env.ORYNX_BACKEND || "http://127.0.0.1:8080";
const EMAIL = process.env.ORYNX_OPERATOR_EMAIL || "inder.singh@thecloudmantra.com";
const CODE = process.env.ORYNX_DEV_CODE || "888888";
const PORT = Number(process.env.ORYNX_AUTHGATE_PORT || 4000);

function postJSON(path, body) {
  return new Promise((resolve, reject) => {
    const data = Buffer.from(JSON.stringify(body));
    const u = new URL(BACKEND + path);
    const req = http.request(
      {
        hostname: u.hostname,
        port: u.port,
        path: u.pathname,
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Content-Length": data.length,
        },
      },
      (res) => {
        let buf = "";
        res.on("data", (c) => (buf += c));
        res.on("end", () => resolve({ status: res.statusCode, headers: res.headers, body: buf }));
      }
    );
    req.on("error", reject);
    req.write(data);
    req.end();
  });
}

async function mintSession() {
  // send-code seeds the dev verification code; verify-code exchanges it for a
  // 30-day session. send-code is rate-limited (1/min); a 429 is harmless as
  // long as a recent code is still valid, so its failure is non-fatal.
  await postJSON("/auth/send-code", { email: EMAIL }).catch(() => {});
  const r = await postJSON("/auth/verify-code", { email: EMAIL, code: CODE });
  return r;
}

const server = http.createServer(async (req, res) => {
  try {
    let r = await mintSession();
    // One retry in case the code was already consumed by a concurrent hit.
    if (r.status !== 200 || !r.headers["set-cookie"]) {
      r = await mintSession();
    }
    const cookies = r.headers["set-cookie"];
    if (r.status === 200 && cookies) {
      res.setHeader("Set-Cookie", cookies);
      res.writeHead(302, { Location: "/" });
      res.end();
      return;
    }
    res.writeHead(502, { "Content-Type": "text/plain" });
    res.end("orynx auto-login failed (backend status " + r.status + ")");
  } catch (e) {
    res.writeHead(502, { "Content-Type": "text/plain" });
    res.end("orynx auto-login error: " + e.message);
  }
});

server.listen(PORT, "127.0.0.1", () => {
  console.log("orynx auto-login bridge listening on 127.0.0.1:" + PORT);
});
