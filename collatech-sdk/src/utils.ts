// ---------------------------------------------------------------------------
// CollaTech Agent SDK – Utility Helpers
// ---------------------------------------------------------------------------

/** Strip data-URI prefix from a base64 string if present. */
export function stripDataUri(base64: string): string {
  const i = base64.indexOf(",");
  if (base64.startsWith("data:") && i >= 0) {
    return base64.slice(i + 1);
  }
  return base64;
}

/** Convert an ArrayBuffer or Uint8Array to a base64 string. */
export function toBase64(buffer: ArrayBuffer | Uint8Array): string {
  const bytes =
    buffer instanceof Uint8Array ? buffer : new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

/** Convert a base64 string to a Uint8Array. */
export function fromBase64(base64: string): Uint8Array {
  const binary = atob(stripDataUri(base64));
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

/** Read a File or Blob as a base64 data-URI string (browser). */
export function fileToBase64(file: File | Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(new Error("Failed to read file"));
    reader.readAsDataURL(file);
  });
}

/** Normalise a base URL (strip trailing slash). */
export function normalizeUrl(url: string): string {
  return url.replace(/\/+$/, "");
}
