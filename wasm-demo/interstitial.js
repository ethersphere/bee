// Interstitial script — runs on the "Bee node starting…" page that the
// service worker serves while the in-worker bee node is still warming up
// (no peers / startup-grace not yet elapsed). Polls /__bee_ready and, when
// the SW reports ready, navigates to the page so the SW's navigation
// handler forwards to bee for the real content.
//
// Parameters come from this script tag's own src query string, e.g.
//   <script src="/interstitial.js?m=2&g=20000"></script>
//   m = minimum peer count (informational; the SW enforces the actual gate)
//   g = startup grace ms (informational)
(async () => {
  const params = new URL(document.currentScript.src).searchParams;
  const MIN = parseInt(params.get("m") || "2", 10);
  const s = document.getElementById("s");

  function setStatus(text) {
    s.textContent = text;
    console.log("[bzz-page]", text);
  }

  function statusFor(body) {
    if (body.reason === "grace") {
      const sec = Math.ceil((body.graceRemaining || 0) / 1000);
      return `Warming up bee node (${sec}s, ${body.peers} peers so far)…`;
    }
    if (body.reason === "peers") {
      return `Connecting to Swarm: ${body.peers} / ${MIN} peers…`;
    }
    if (body.reason === "api") {
      return "Bee API not ready yet…";
    }
    return `Peers: ${body.peers} / ${MIN}`;
  }

  // Poll until the SW reports ready, then reload. The SW's navigation
  // handler will see the reload as a top-level navigation; since readiness
  // is now true, it'll forward the request to bee and return the content
  // (or, if the underlying retrieval fails, fall back to this interstitial
  // again — at which point we just keep polling).
  for (;;) {
    await new Promise((r) => setTimeout(r, 2000));
    let body;
    try {
      const r = await fetch("/__bee_ready", { cache: "no-store" });
      if (!r.ok) {
        setStatus(`Bee API not ready yet (${r.status})…`);
        continue;
      }
      body = await r.json();
    } catch {
      setStatus("Waiting for service worker…");
      continue;
    }
    setStatus(statusFor(body));
    if (body.ready) {
      // Bee is ready: navigate so the SW handles the request as a real top-
      // level GET. The SW already runs the readiness check in its navigation
      // handler, so if bee actually returns 200 we render the content; if
      // not, the SW serves the interstitial again and we keep polling.
      location.reload();
      return;
    }
  }
})();
