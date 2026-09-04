export interface Summary {
  total: number;
  added: number;
  removed: number;
  modified: number;
  by_category: Record<string, number>;
}

export interface DiffSummary {
  id: string;
  detected_at: string;
  old_serial: string;
  new_serial: string;
  summary: Summary;
}

export interface Change {
  kind: "added" | "removed" | "modified";
  category: string;
  name: string;
  type: string;
  old_ttl?: number;
  new_ttl?: number;
  old_rdata?: string;
  new_rdata?: string;
}

export interface DiffEntry extends DiffSummary {
  changes: Change[];
  /** ページング時のみ返される。フィルタ適用後の全 changes 件数 */
  changes_total?: number;
  page?: number;
  per_page?: number;
  total_pages?: number;
}

export interface DiffListResponse {
  diffs: DiffSummary[];
  total: number;
  page: number;
  per_page: number;
}

export interface DetailQuery {
  category?: string;
  page?: number;
  perPage?: number;
}

async function getJSON<T>(url: string): Promise<T> {
  const resp = await fetch(url);
  if (!resp.ok) {
    let message = `HTTP ${resp.status}`;
    try {
      const body = (await resp.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      // JSON 以外のエラーレスポンスはステータスコードのまま扱う
    }
    throw new Error(message);
  }
  return (await resp.json()) as T;
}

function detailQueryString(query?: DetailQuery): string {
  if (!query) return "";
  const params = new URLSearchParams();
  if (query.category && query.category !== "all") {
    params.set("category", query.category);
  }
  if (query.page !== undefined) params.set("page", String(query.page));
  if (query.perPage !== undefined) params.set("per_page", String(query.perPage));
  const s = params.toString();
  return s ? `?${s}` : "";
}

export function fetchDiffs(page: number, perPage: number): Promise<DiffListResponse> {
  return getJSON(`/api/diffs?page=${page}&per_page=${perPage}`);
}

export function fetchDiff(id: string, query?: DetailQuery): Promise<DiffEntry> {
  return getJSON(`/api/diffs/${encodeURIComponent(id)}${detailQueryString(query)}`);
}

// root anchors の差分は zone と JSON 形状が同じ (serial フィールドに TrustAnchor id が入る)。
export function fetchAnchorDiffs(page: number, perPage: number): Promise<DiffListResponse> {
  return getJSON(`/api/anchors/diffs?page=${page}&per_page=${perPage}`);
}

export function fetchAnchorDiff(id: string, query?: DetailQuery): Promise<DiffEntry> {
  return getJSON(`/api/anchors/diffs/${encodeURIComponent(id)}${detailQueryString(query)}`);
}

export function formatDetectedAt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    timeZoneName: "short",
  });
}
