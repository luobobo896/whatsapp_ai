import { pathToFileURL } from "node:url";

const modulePath = process.argv[2];
const accountId = process.argv[3];

// Capture the real stdout so only emit() can write clean JSON.
// OpenClaw's module writes a TUI (banner + ASCII QR) to stdout on import
// and during startWebLoginWithQr — suppress that noise.
const rawStdout = process.stdout.write.bind(process.stdout);
process.stdout.write = () => true;
console.log = () => {};
console.error = () => {};

function emit(event) {
  rawStdout(`${JSON.stringify(event)}\n`);
}

try {
  const { startWebLoginWithQr, waitForWebLogin } = await import(pathToFileURL(modulePath).href);
  const first = await startWebLoginWithQr({
    accountId,
    force: true,
    timeoutMs: 20000,
  });

  // Restore in case the OpenClaw function itself needs to write (it shouldn't,
  // but be safe), and re-suppress before each waitForWebLogin call below.
  process.stdout.write = () => true;

  if (!first.qrDataUrl) {
    if (first.connected) {
      emit({ type: "status", connected: true, message: first.message });
      process.exit(0);
    }
    throw new Error(first.message || "OpenClaw did not return a WhatsApp QR code.");
  }

  let currentQrDataUrl = first.qrDataUrl;
  emit({ type: "qr", qrDataUrl: currentQrDataUrl, message: first.message });

  while (true) {
    const result = await waitForWebLogin({
      accountId,
      currentQrDataUrl,
      timeoutMs: 35000,
    });
    if (result.qrDataUrl) {
      currentQrDataUrl = result.qrDataUrl;
      emit({ type: "qr", qrDataUrl: currentQrDataUrl, message: result.message });
      continue;
    }
    emit({ type: "status", connected: Boolean(result.connected), message: result.message });
    process.exit(result.connected ? 0 : 1);
  }
} catch (error) {
  emit({ type: "error", error: error instanceof Error ? error.message : String(error) });
  process.exit(1);
}
