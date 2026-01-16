import "./wasm_exec.js";

import { configure, fs, InMemory } from "@zenfs/core";

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

self.addEventListener("fetch", (e) => {
  const { pathname } = new URL(e.request.url);
  if (!pathname.startsWith(path)) return;

  e.respondWith(handlerPromise.then((handler) => handler(e.request)));
});

async function main() {
  // Set up ZenFS in-memory filesystem
  await configure({
    mounts: {
      "/tmp": InMemory,
      "/home/user": InMemory,
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
    "--bootnode",
    "/dnsaddr/testnet.ethswarm.org",
    "--data-dir",
    "/home/user/.bee/sepolia",
    "--verbosity",
    "debug",
    // '--blockchain-rpc-endpoint',
    // 'https://ethereum-sepolia-rpc.publicnode.com/ac5b7f52aabd778861c2588f872f15c5fc34f0b343ec3d18ac2e91f5526e9c2b',
    "--mainnet=false",
    "--network-id=5",
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
