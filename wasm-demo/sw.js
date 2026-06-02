import "./wasm_exec.js";

import { configure, fs, InMemory } from "@zenfs/core";
import { IndexedDB } from "@zenfs/dom";

const WASM_PATH = "bee.wasm";

self.addEventListener("activate", (event) => {
  event.waitUntil(
    clients.claim().catch((error) => {
      console.error("Error during service worker activate:", error);
    }),
  );
});

// Handle fetch events to serve the WASM module if needed
const path = new URL(self.registration.scope).pathname;
const handlerPromise = new Promise((setHandler) => {
  self.wasmhttp = { path, setHandler };
});

// --- Subdomain gateway support -------------------------------------------
// Mirrors the IPFS service-worker-gateway: when this worker runs on
// "<reference>.localhost" (or "<reference>.bzz.localhost"), every request
// is served from the in-worker bee node as "/bzz/<reference>/<path>".
//
// Also supports "<reference>.bytes.localhost", which routes to bee's raw
// chunk endpoint "/bytes/<reference>" — useful when the address points to a
// single chunk (not a manifest). bee's /bzz/ handler always tries to parse
// the chunk as a manifest, so for raw bytes you must use /bytes/.
//
// The leftmost label is treated as the bzz reference. Only applied to named
// hosts (not bare IPs), since IPs cannot have subdomains.
function getSubdomainTarget() {
  const host = self.location.hostname;
  if (host === "localhost" || !host.endsWith(".localhost")) return null;
  let label = host.slice(0, -".localhost".length); // strip trailing ".localhost"

  // Determine which bee endpoint the subdomain is targeting.
  let endpoint = "bzz";
  if (label.endsWith(".bytes")) {
    endpoint = "bytes";
    label = label.slice(0, -".bytes".length);
  } else if (label.endsWith(".bzz")) {
    endpoint = "bzz";
    label = label.slice(0, -".bzz".length);
  }
  // keep only the leftmost label (handles ".bzz"/".bytes" infix + stray dots)
  label = label.split(".").pop();
  if (!label) return null;
  return { reference: label, endpoint };
}

const subdomainTarget = getSubdomainTarget();
const bzzReference = subdomainTarget?.reference ?? null;
// Prefix is "/bzz/<ref>" for manifest-style routing or "/bytes/<ref>" for raw
// chunk routing. The /bytes/ endpoint takes no further path component.
const bzzPrefix = subdomainTarget
  ? `/${subdomainTarget.endpoint}/${subdomainTarget.reference}`
  : null;
const isBytesEndpoint = subdomainTarget?.endpoint === "bytes";

// Minimum connected peer count before we let the page try to retrieve content.
// Issuing retrieval with 0 peers just produces "no peers left" 404s; waiting
// for a couple of peers makes the very first navigation actually succeed.
const MIN_PEERS_FOR_RETRIEVAL = 2;

// Bee enforces NO warmup delay for light nodes (pkg/node/node.go: light nodes
// get warmupTime = 0). So even with --warmup-time set, ultra-light bee will
// happily serve the API as soon as init finishes, before kademlia has had time
// to populate routing bins beyond bootnodes. We add the grace ourselves: the
// SW refuses to declare readiness until this many ms have passed since the
// worker booted, giving kademlia time to discover + connect to neighbours.
const STARTUP_GRACE_MS = 20_000;
const swStartTime = Date.now();

// How long to wait on the final content retrieval before aborting and retrying.
// Bee retrieval can be slow under accounting throttling / sparse peer set, so
// generous timeout — but not infinite, so a stuck request doesn't freeze the UI.
const FETCH_TIMEOUT_MS = 60_000;

// Interstitial HTML — shown while bee is still warming up (grace / peers / 503).
// The bootstrap script (interstitial.js) polls /__bee_ready and reloads once
// the SW reports ready. The reload triggers a real navigation through the SW,
// which forwards to bee for the actual content.
const STARTING_PAGE =
  '<!doctype html><html lang="en"><head><meta charset="utf-8">' +
  '<meta name="viewport" content="width=device-width,initial-scale=1">' +
  "<title>Starting…</title><style>body{font-family:system-ui,sans-serif;background:#0b0b0b;color:#eee;" +
  "display:grid;place-items:center;height:100vh;margin:0}.b{text-align:center;max-width:32rem;padding:1rem}" +
  ".s{font-size:2.5rem;animation:p 1.5s ease-in-out infinite}@keyframes p{0%,100%{opacity:.35}50%{opacity:1}}" +
  ".m{margin-top:1rem;opacity:.7;font-size:.9rem}</style>" +
  '</head><body><div class="b"><div class="s">🐝</div><h2>Bee node starting…</h2>' +
  '<p>Connecting to Swarm peers.</p><p class="m" id="s">Initialising…</p></div>' +
  '<script src="/interstitial.js?m=' + MIN_PEERS_FOR_RETRIEVAL +
  "&g=" + STARTUP_GRACE_MS + '"></script></body></html>';

// HTML-escape user-controlled text for safe inclusion in the failure page.
function escHtml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

// Build a terminal failure page that shows bee's actual response body so the
// user sees what really went wrong. The previous hardcoded "chunks not
// reachable" message lied for any 404 that wasn't actually a peer/retrieval
// issue (e.g. "path address not found" when the manifest loaded fine, or
// "invalid path" on CID validation).
function buildErrorPage({ status, beeMessage, endpoint }) {
  const titleByStatus = {
    404: "Not found",
    400: "Bad request",
    500: "Server error",
  };
  const title = titleByStatus[status] || `Bee returned ${status}`;
  return (
    '<!doctype html><html lang="en"><head><meta charset="utf-8">' +
    '<meta name="viewport" content="width=device-width,initial-scale=1">' +
    "<title>" + escHtml(title) + "</title>" +
    "<style>body{font-family:system-ui,sans-serif;background:#0b0b0b;" +
    "color:#eee;display:grid;place-items:center;min-height:100vh;margin:0;padding:2rem;box-sizing:border-box}" +
    ".b{text-align:center;max-width:42rem;padding:1rem}" +
    ".s{font-size:2.5rem}.code{font-size:.85rem;opacity:.85;margin-top:1rem;" +
    "background:#1a1a1a;border:1px solid #2a2a2a;border-radius:.25rem;" +
    "padding:.6rem .8rem;text-align:left;white-space:pre-wrap;word-break:break-word;" +
    "font-family:ui-monospace,monospace}.m{margin-top:1rem;opacity:.7;font-size:.9rem}" +
    "button{margin-top:1.5rem;padding:.6rem 1.2rem;background:#333;color:#eee;" +
    "border:1px solid #555;border-radius:.25rem;cursor:pointer;font:inherit}" +
    "button:hover{background:#444}</style>" +
    '</head><body><div class="b"><div class="s">🕸️</div>' +
    "<h2>" + escHtml(title) + "</h2>" +
    "<p>Bee at <code>" + escHtml(endpoint) + "</code> returned HTTP " +
    escHtml(String(status)) + ".</p>" +
    '<div class="code">' + escHtml(beeMessage || "(no message body)") + "</div>" +
    '<p class="m">Common causes: the chunk or one of its sub-chunks is not ' +
    "reachable through the current wss peer set; the address points to a " +
    "manifest path that doesn't exist; the address is a single raw chunk and " +
    "needs the <code>.bytes</code> subdomain form instead of <code>.bzz</code>.</p>" +
    '<button onclick="location.reload()">Retry</button></div></body></html>'
  );
}



// Probe bee for liveness + peer count. We deliberately do NOT do a HEAD on
// /bzz/<ref>/ here: HEAD walks the entire manifest path to resolve the index
// document, so it 404s if *any* chunk in that walk is unreachable through the
// current peer set — which leaves the page stuck waiting on a condition that
// the eventual GET would face anyway. Instead, once peers ≥ MIN we let the
// interstitial issue the GET and surface the real outcome (success, partial,
// or honest 404 with a retry counter).
async function beeReadiness(handler) {
  const t0 = Date.now();
  const elapsed = Date.now() - swStartTime;
  const graceRemaining = Math.max(0, STARTUP_GRACE_MS - elapsed);

  try {
    const peersUrl = new URL(self.location.origin + "/peers");
    const pr = await handler(new Request(peersUrl, { method: "GET" }));
    if (pr.status !== 200) {
      const r = {
        peers: 0,
        ready: false,
        status: pr.status,
        reason: "api",
        graceRemaining,
      };
      console.log("[bzz-sw] readiness:", JSON.stringify(r), "in", Date.now() - t0, "ms");
      return r;
    }
    const pj = await pr.json();
    const peers = Array.isArray(pj?.peers) ? pj.peers.length : 0;

    // Both gates must pass: enough peers AND past the SW-side startup grace.
    // Bee gives light nodes warmupTime=0, so without this grace, kademlia bin
    // population hasn't caught up by the time we'd try retrieval.
    let ready, reason;
    if (graceRemaining > 0) {
      ready = false;
      reason = "grace";
    } else if (peers < MIN_PEERS_FOR_RETRIEVAL) {
      ready = false;
      reason = "peers";
    } else {
      ready = true;
      reason = "ok";
    }

    const r = { peers, ready, status: 200, reason, graceRemaining };
    console.log("[bzz-sw] readiness:", JSON.stringify(r), "in", Date.now() - t0, "ms");
    return r;
  } catch (e) {
    const r = {
      peers: 0,
      ready: false,
      status: 0,
      reason: "api",
      err: String(e),
      graceRemaining,
    };
    console.log("[bzz-sw] readiness:", JSON.stringify(r));
    return r;
  }
}

self.addEventListener("fetch", (e) => {
  const url = new URL(e.request.url);
  if (!url.pathname.startsWith(path)) return;

  if (!bzzReference) {
    e.respondWith(handlerPromise.then((handler) => handler(e.request)));
    return;
  }

  // Virtual endpoint the interstitial polls to decide when to navigate.
  if (url.pathname === "/__bee_ready") {
    e.respondWith(
      handlerPromise.then(async (handler) => {
        const j = await beeReadiness(handler);
        return new Response(JSON.stringify(j), {
          status: 200,
          headers: {
            "content-type": "application/json",
            "cache-control": "no-store",
          },
        });
      }),
    );
    return;
  }

  // Passthrough for the interstitial bootstrap script: served as a static
  // file by the dev server at the root origin, but the subdomain origin
  // resolves to loopback too, so an absolute fetch by the SW reaches it.
  if (url.pathname === "/interstitial.js") {
    // Drop the SW from the loop by re-fetching with the script's own URL.
    // The browser will route this directly to the network (python http.server)
    // because the request comes from the SW global, not a controlled client.
    e.respondWith(fetch(e.request.url, { cache: "no-store" }));
    return;
  }

  // Rewrite the request path onto bee's API.
  //   /bzz/<ref>:   prefix and append the user's path (manifest semantics)
  //   /bytes/<ref>: the endpoint takes no path; any user path is meaningless,
  //                 just return the raw chunk(s) for the address.
  if (isBytesEndpoint) {
    url.pathname = bzzPrefix; // exactly /bytes/<ref>
  } else {
    url.pathname = `${bzzPrefix}${url.pathname}`;
  }
  const rewritten = new Request(url.toString(), {
    method: e.request.method,
    headers: e.request.headers,
    redirect: "manual",
  });

  // Keep the worker alive a bit longer than the response itself, so bee's
  // **background** cache-write goroutines (pkg/storer/netstore.go schedules
  // Cache().Put in a goroutine after the chunk is returned) have time to
  // land in IndexedDB. Without this, Chrome can idle-kill the SW once the
  // response stream closes — losing in-flight leveldb memtable writes and
  // forcing the next reload to re-fetch the same chunks from the network.
  // 10s is generous; the cacheLimiter has bounded concurrency.
  if (e.request.mode === "navigate") {
    e.waitUntil(new Promise((res) => setTimeout(res, 10_000)));
  }

  e.respondWith(
    handlerPromise
      .then(async (handler) => {
        // For top-level navigations: only proceed with retrieval once bee
        // has the API up AND enough peers. Otherwise the page sees the
        // interstitial, which polls /__bee_ready and reloads when ready.
        // This avoids the auto-refresh racing against in-flight chunk
        // retrievals (which caused "context canceled" mid-fetch and
        // half-loaded pages without stylesheets).
        if (e.request.mode === "navigate") {
          const ready = await beeReadiness(handler);
          if (!ready.ready) {
            return new Response(STARTING_PAGE, {
              status: 200,
              headers: { "content-type": "text/html; charset=utf-8" },
            });
          }
        }
        return handler(rewritten);
      })
      .then(async (resp) => {
        // Backstop for top-level navigations:
        //   503    → bee says "syncing" — serve warming interstitial.
        //   other  → if not 2xx and the user expects a renderable page,
        //            surface bee's actual error body instead of misleading
        //            "content not reachable" verbiage. We read the JSON
        //            error message bee returns and show it verbatim.
        if (e.request.mode === "navigate") {
          if (resp.status === 503) {
            return new Response(STARTING_PAGE, {
              status: 200,
              headers: { "content-type": "text/html; charset=utf-8" },
            });
          }
          if (resp.status >= 400) {
            // Drain bee's JSON-ish error body to extract the message field
            // and show it. We clone first so we don't consume a body that
            // might still need to be returned in a degenerate case.
            let beeMessage = "";
            try {
              const text = await resp.clone().text();
              if (text) {
                try {
                  const j = JSON.parse(text);
                  beeMessage = j.message || text;
                  if (Array.isArray(j.reasons) && j.reasons.length) {
                    beeMessage += "\n\n" + JSON.stringify(j.reasons, null, 2);
                  }
                } catch {
                  beeMessage = text;
                }
              }
            } catch {}
            return new Response(
              buildErrorPage({
                status: resp.status,
                beeMessage,
                endpoint: bzzPrefix,
              }),
              {
                status: 200,
                headers: { "content-type": "text/html; charset=utf-8" },
              },
            );
          }
        }

        // Keep bee's internal redirects (e.g. trailing-slash, manifest paths)
        // on the subdomain by stripping the "/bzz/<ref>" prefix from Location.
        const loc = resp.headers.get("location");
        if (!loc) return resp;
        let next = loc;
        try {
          const locUrl = new URL(loc, url);
          if (locUrl.pathname.startsWith(bzzPrefix)) {
            locUrl.pathname = locUrl.pathname.slice(bzzPrefix.length) || "/";
            next = locUrl.pathname + locUrl.search + locUrl.hash;
          }
        } catch {}
        if (next === loc) return resp;
        const headers = new Headers(resp.headers);
        headers.set("location", next);
        return new Response(resp.body, {
          status: resp.status,
          statusText: resp.statusText,
          headers,
        });
      }),
  );
});

async function main() {
  // ZenFS mount layout:
  //   /tmp                                  -> InMemory (ephemeral scratch)
  //   /home/user                            -> InMemory (keys + state, dies with the SW)
  //   /home/user/.bee/mainnet/localstore    -> IndexedDB (chunk store, persists)
  //
  // Putting just `localstore/` on IndexedDB caches retrieved chunks + their
  // leveldb index across reloads, so a refresh doesn't re-fetch the same
  // chunks from the network. Identity/kademlia/accounting state stay in
  // memory (so they don't accidentally become persistent). The IndexedDB
  // store can be cleared via DevTools → Application → Clear storage.
  //
  // Falls back to InMemory if IndexedDB is unavailable (private browsing,
  // unusual SW contexts) so the demo still works, just without chunk caching.
  const LOCALSTORE_PATH = "/home/user/.bee/mainnet/localstore";
  const IDB_STORE_NAME = "bee-localstore-mainnet";

  let localstoreBackend = InMemory;
  try {
    if (
      typeof indexedDB !== "undefined" &&
      (await IndexedDB.isAvailable({ idbFactory: indexedDB }))
    ) {
      localstoreBackend = { backend: IndexedDB, storeName: IDB_STORE_NAME };
      console.log("ZenFS: localstore mounted on IndexedDB (" + IDB_STORE_NAME + ")");
    } else {
      console.warn("ZenFS: IndexedDB unavailable, localstore will be in-memory");
    }
  } catch (err) {
    console.warn("ZenFS: IndexedDB probe failed, falling back to InMemory", err);
  }

  await configure({
    mounts: {
      "/tmp": InMemory,
      "/home/user": InMemory,
      [LOCALSTORE_PATH]: localstoreBackend,
    },
  });

  // Create necessary directories
  await fs.promises.mkdir("/home/user/.bee/keys", {
    recursive: true,
    mode: 0o700,
  });

  // Write key files
  await fs.promises.writeFile(
    "/home/user/.bee/keys/libp2p_v2.key",
    JSON.stringify({
      address:
        "049886e5793c6261f59e7b047a91c27226cdbc2ba5af60c9e26705c15441ec9e3f7daa7085a2a7665c338171eb2bf1b65a173636137405d825d0385bc4defacaf4",
      crypto: {
        cipher: "aes-128-ctr",
        ciphertext:
          "e35f6f83893bc6186119b85244b43d42b08f92891b6cb7c81f695c0a94ea2536c84fb84e3410618ddee7c814acdf35f1facc79597540e6fa3d460278ffa414311880676ef5fad8b06362b422c139ffb5cdbad530d371e645dc8e496b7b04f93c2ae23554cfc1452a414bf0c1324d326d45980d190ff784ebd9",
        cipherparams: { iv: "f917c56ec7e2aa36fd592c63894aa18a" },
        kdf: "scrypt",
        kdfparams: {
          n: 32768,
          r: 8,
          p: 1,
          dklen: 32,
          salt: "dcbc48279045788f9b12ffa7989880290b190e50506e2d9596b4d476528cedd0",
        },
        mac: "1482a352544e9cc13c1954acf9c313c9e25901c530262c7e198d4f221b76027a",
      },
      version: 3,
      id: "5117e84d-0a2b-4c4c-808d-1e9676903c8a",
    }),
    { mode: 0o600 },
  );

  await fs.promises.writeFile(
    "/home/user/.bee/keys/swarm.key",
    JSON.stringify({
      address: "ed48f21d97fd09d08584f42c97f737bc549c49bf",
      crypto: {
        cipher: "aes-128-ctr",
        ciphertext:
          "85221a9ec6ff8328f80686ddaa6afe9c1da4b74e8494515cc34e1ff2b9567285",
        cipherparams: { iv: "e01d72acdfb68338adcf99ae44f7aeb0" },
        kdf: "scrypt",
        kdfparams: {
          n: 32768,
          r: 8,
          p: 1,
          dklen: 32,
          salt: "7ac4dd27cfe9b796793270a6b4c84e9f717533161105a6425576da20aff0f554",
        },
        mac: "ccaa689b4f09bab5580b515dfcb1a6fcbc7ced6d769a5b8232b1508eaa9c6dc3",
      },
      version: 3,
      id: "2f567b5f-122d-4625-a9f5-25c3285550e1",
    }),
    { mode: 0o600 },
  );

  // Expose ZenFS for debugging purposes
  self.ZenFS = fs;

  // Initialize Go runtime and set environment variables
  const go = new Go();

  go.env = {
    HOME: "/home/user",
    PATH: "/usr/bin:/usr/local/bin",
  };

  go.argv = [
    "bee.wasm",
    "start",
    "--password",
    "testing",
    // Mainnet bootnode. The dnsaddr is resolved in-browser via the DoH
    // resolver in pkg/p2p/discover_js.go, yielding the libp2p.direct AutoTLS
    // wss addresses the wasmws transport can dial. More mainnet nodes now
    // advertise wss, so a single dnsaddr gives broader bin coverage than
    // pinning a fixed list.
    "--bootnode",
    "/dnsaddr/mainnet.ethswarm.org",
    "--data-dir",
    "/home/user/.bee/mainnet",
    "--verbosity",
    "debug",
    // '--blockchain-rpc-endpoint',
    // 'https://rpc.gnosischain.com',
    "--mainnet=true",
    "--network-id=1",
    "--p2p-ws-enable",
  ];

  // Override Go's `fs.readFile` with ZenFS readFile functionality
  const goImportObject = {
    ...go.importObject,
    fs: {
      ...fs,
      readFile: async (path) => {
        try {
          const data = await fs.promises.readFile(path, "utf8");
          return new TextEncoder().encode(data); // Return Uint8Array
        } catch (error) {
          console.error("Error reading file:", path, error);
          throw error;
        }
      },
    },
  };

  // Load and run the WASM binary
  await WebAssembly.instantiateStreaming(fetch(WASM_PATH), goImportObject).then(
    (result) => {
      go.run(result.instance);
    },
  );
}

main().catch((error) => {
  console.error("Error running worker:", error);
});
