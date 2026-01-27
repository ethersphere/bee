if ("serviceWorker" in navigator) {
  navigator.serviceWorker
    .register("/sw_bundle.js", { scope: "/" })
    .then((registration) => {
      console.log("Service Worker registered successfully:", registration);
    })
    .catch((error) => {
      console.error("Service Worker registration failed:", error);
    });
} else {
  console.log("Service Worker is not supported in this browser.");
}
