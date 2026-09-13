import { normalizeLinkKind, type LinkKind } from "./link-kinds";

export type ImportedLink = {
  label: string;
  url: string;
  kind: LinkKind;
  thumbnail_url: string;
  note?: string;
};

export type ImportParseResult = {
  rows: ImportedLink[];
  errors: string[];
};

const MAX_IMPORT_LINKS = 50;
const URL_HEADERS = new Set(["url", "link", "href", "destination"]);
const LABEL_HEADERS = new Set(["label", "title", "name", "text"]);
const KIND_HEADERS = new Set(["kind", "type", "platform"]);
const IMAGE_HEADERS = new Set(["thumbnail", "thumbnail_url", "image", "image_url"]);

export function parseLinkImport(source: string): ImportParseResult {
  const lines = source
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  if (lines.length === 0) return { rows: [], errors: [] };

  const delimiter = detectDelimiter(lines[0]);
  const records = delimiter ? lines.map((line) => parseDelimitedLine(line, delimiter)) : [];
  const header = records.length > 0 ? records[0].map(normalizeHeader) : [];
  const hasHeader = header.some((cell) => URL_HEADERS.has(cell));
  const dataLines = hasHeader ? lines.slice(1) : lines;
  const dataRecords = hasHeader ? records.slice(1) : records;
  const rows: ImportedLink[] = [];
  const errors: string[] = [];

  for (let index = 0; index < dataLines.length; index += 1) {
    if (rows.length >= MAX_IMPORT_LINKS) {
      errors.push(`Only the first ${MAX_IMPORT_LINKS} valid links are imported at once.`);
      break;
    }
    const line = dataLines[index];
    const record = delimiter ? dataRecords[index] : undefined;
    const row = hasHeader && record
      ? rowFromHeader(record, header)
      : record && record.length > 1
        ? rowFromColumns(record)
        : rowFromFreeform(line);
    if (!row) {
      errors.push(`Line ${index + 1 + (hasHeader ? 1 : 0)} did not contain a valid http(s) URL.`);
      continue;
    }
    rows.push(row);
  }
  return { rows, errors };
}

export function inferLinkKind(value: string, fallback: LinkKind = "website"): LinkKind {
  try {
    const host = new URL(value).hostname.toLowerCase().replace(/^www\./, "");
    if (host === "github.com") return "github";
    if (host === "youtu.be" || host === "youtube.com" || host.endsWith(".youtube.com")) return "youtube";
    if (host === "instagram.com" || host.endsWith(".instagram.com")) return "instagram";
    if (host === "tiktok.com" || host.endsWith(".tiktok.com")) return "tiktok";
    if (host === "x.com" || host.endsWith(".x.com") || host === "twitter.com" || host.endsWith(".twitter.com")) return "x";
    if (host === "bsky.app" || host.endsWith(".bsky.app")) return "bluesky";
    if (host === "linkedin.com" || host.endsWith(".linkedin.com")) return "linkedin";
    if (host === "spotify.com" || host.endsWith(".spotify.com")) return "spotify";
  } catch {
    return fallback;
  }
  return fallback;
}

export function providerKind(provider: string | undefined, url: string): LinkKind {
  const normalized = normalizeLinkKind(provider);
  return normalized === "website" ? inferLinkKind(url, "website") : normalized;
}

export function genericLabelForURL(value: string) {
  try {
    return new URL(value).hostname.replace(/^www\./, "");
  } catch {
    return "Link";
  }
}

function rowFromHeader(record: string[], header: string[]): ImportedLink | null {
  const value = (headers: Set<string>) => {
    const index = header.findIndex((cell) => headers.has(cell));
    return index >= 0 ? record[index]?.trim() ?? "" : "";
  };
  const url = normalizeURL(value(URL_HEADERS));
  if (!url) return null;
  const label = value(LABEL_HEADERS) || genericLabelForURL(url);
  const kindValue = value(KIND_HEADERS);
  return {
    label,
    url,
    kind: kindValue ? normalizeLinkKind(kindValue.toLowerCase()) : inferLinkKind(url),
    thumbnail_url: normalizeOptionalURL(value(IMAGE_HEADERS)),
  };
}

function rowFromColumns(record: string[]): ImportedLink | null {
  const urlIndex = record.findIndex((cell) => normalizeURL(cell));
  if (urlIndex < 0) return null;
  const url = normalizeURL(record[urlIndex]);
  if (!url) return null;
  const label = record.find((cell, index) => index !== urlIndex && cell.trim() && !normalizeURL(cell))?.trim() || genericLabelForURL(url);
  const kindCandidate = record[urlIndex + 1]?.trim().toLowerCase();
  const imageCandidate = record[urlIndex + 2]?.trim() ?? "";
  return {
    label,
    url,
    kind: kindCandidate ? normalizeLinkKind(kindCandidate) : inferLinkKind(url),
    thumbnail_url: normalizeOptionalURL(imageCandidate),
  };
}

function rowFromFreeform(line: string): ImportedLink | null {
  const match = line.match(/https?:\/\/[^\s]+/i);
  if (!match) return null;
  const raw = match[0].replace(/[),.;]+$/, "");
  const url = normalizeURL(raw);
  if (!url) return null;
  const label = line
    .slice(0, match.index ?? 0)
    .replace(/[|,:\-–—>]+$/g, "")
    .trim() || genericLabelForURL(url);
  return { label, url, kind: inferLinkKind(url), thumbnail_url: "" };
}

function normalizeURL(value: string | undefined) {
  if (!value) return "";
  try {
    const parsed = new URL(value.trim());
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return "";
    return parsed.toString();
  } catch {
    return "";
  }
}

function normalizeOptionalURL(value: string | undefined) {
  return normalizeURL(value) || "";
}

function normalizeHeader(value: string) {
  return value.trim().toLowerCase().replace(/[\s-]+/g, "_");
}

function detectDelimiter(line: string) {
  for (const delimiter of ["\t", "|", ","]) {
    if (line.includes(delimiter)) return delimiter;
  }
  return "";
}

function parseDelimitedLine(line: string, delimiter: string) {
  const cells: string[] = [];
  let value = "";
  let quoted = false;
  for (let index = 0; index < line.length; index += 1) {
    const char = line[index];
    if (char === '"') {
      if (quoted && line[index + 1] === '"') {
        value += '"';
        index += 1;
      } else {
        quoted = !quoted;
      }
      continue;
    }
    if (char === delimiter && !quoted) {
      cells.push(value.trim());
      value = "";
      continue;
    }
    value += char;
  }
  cells.push(value.trim());
  return cells;
}
