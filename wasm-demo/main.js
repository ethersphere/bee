// On "<reference>.localhost" subdomains the service worker serves the bzz
// content in place. The first hit has no controlling SW yet (so the dev server
// returns this bootstrap page), so once the SW is registered and controlling
// the page we reload to let it serve "/bzz/<reference>/".
const onSubdomain =
  location.hostname !== "localhost" && location.hostname.endsWith(".localhost");

if ("serviceWorker" in navigator) {
  navigator.serviceWorker
    .register("/sw_bundle.js", { scope: "/" })
    .then(() => {
      console.log("Service Worker registered successfully");

      if (!onSubdomain) return;

      if (navigator.serviceWorker.controller) {
        // SW already controls this page: reload so it serves the content.
        location.reload();
      } else {
        // Wait until the freshly-installed SW takes control, then reload once.
        navigator.serviceWorker.addEventListener(
          "controllerchange",
          () => location.reload(),
          { once: true },
        );
      }
    })
    .catch((error) => {
      console.error("Service Worker registration failed:", error);
    });
} else {
  console.log("Service Worker is not supported in this browser.");
}
