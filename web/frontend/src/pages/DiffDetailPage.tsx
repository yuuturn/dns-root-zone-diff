import {
  Banner,
  Breadcrumbs,
  Empty,
  LayerCard,
  Loader,
  Pagination,
  Table,
  Tabs,
  Text,
} from "@cloudflare/kumo";
import { WarningCircleIcon } from "@phosphor-icons/react";
import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import {
  fetchAnchorDiff,
  fetchDiff,
  fetchTLDDiff,
  formatDetectedAt,
  type Change,
  type DetailQuery,
  type DiffEntry,
  type KeyInfo,
} from "../api.ts";
import { CategoryBadge, KindBadge, SummaryBadges } from "../components/badges.tsx";

// 詳細 API の既定ページング件数 (サーバ側 defaultDetailPerPage と一致させる)
const DETAIL_PER_PAGE = 100;

export type DetailVariant = "zone" | "anchors" | "tlds";

// variant ごとの詳細 API。
const detailFetchers: Record<
  DetailVariant,
  (id: string, query?: DetailQuery) => Promise<DiffEntry>
> = {
  zone: fetchDiff,
  anchors: fetchAnchorDiff,
  tlds: fetchTLDDiff,
};

// variant ごとの一覧リンクと serial 相当フィールドの見出し。
const detailLabels: Record<
  DetailVariant,
  { listPath: string; listName: string; previous: string; next: string }
> = {
  zone: { listPath: "/", listName: "Diffs", previous: "Previous serial", next: "New serial" },
  anchors: {
    listPath: "/anchors",
    listName: "Anchors",
    previous: "Previous anchor id",
    next: "New anchor id",
  },
  tlds: {
    listPath: "/tlds",
    listName: "TLDs",
    previous: "Previous version",
    next: "New version",
  },
};

// ALL_TAB は「すべて」タブの値。それ以外は API の category 値をそのまま使う。
const ALL_TAB = "all";

// tabFromHash は URL の #DNSSEC / #glue のようなフラグメントをタブ値に変換する。
// フラグメント無し・空は「すべて」を表す。
function tabFromHash(hash: string): string {
  const raw = hash.startsWith("#") ? hash.slice(1) : hash;
  if (!raw) return ALL_TAB;
  try {
    return decodeURIComponent(raw);
  } catch {
    // 不正なパーセントエンコーディングはそのままタブ値として扱い、
    // 後の resolveTab で有効なカテゴリか判定する。
    return raw;
  }
}

// resolveTab はタブ値が現在のエントリのカテゴリに存在するか検証する。
// 大文字小文字の違い (#dnssec) はカテゴリ名 (#DNSSEC) に寄せ、
// 存在しないカテゴリは「すべて」にフォールバックする。
function resolveTab(tab: string, categories: string[]): string {
  if (tab === ALL_TAB) return ALL_TAB;
  if (categories.includes(tab)) return tab;
  return categories.find((category) => category.toLowerCase() === tab.toLowerCase()) ?? ALL_TAB;
}

// replaceTabHash はタブに対応するフラグメントを URL に書き込む。
// タブはフィルタなので履歴は増やさず、スクロール位置も維持する。
function replaceTabHash(tab: string): void {
  const desiredHash = tab === ALL_TAB ? "" : `#${encodeURIComponent(tab)}`;
  if (window.location.hash === desiredHash) return;
  const nextURL = `${window.location.pathname}${window.location.search}${desiredHash}`;
  window.history.replaceState(window.history.state, "", nextURL);
}

function ChangeTable({ changes }: { changes: Change[] }) {
  if (changes.length === 0) {
    return <Empty size="sm" title="No changes in this category" />;
  }
  return (
    <LayerCard className="p-0">
      <p className="table-scroll-hint">Scroll horizontally to see all columns →</p>
      <div className="table-scroll" role="region" aria-label="Changes table" tabIndex={0}>
        {/* スマートフォンでは列が潰れないよう、テーブル自身に最小幅を持たせて
            カード内で横スクロールさせる (カード側は overflow-hidden のまま)。 */}
        <Table className="diff-table">
          <Table.Header>
            <Table.Row>
              <Table.Head>Kind</Table.Head>
              <Table.Head>Name</Table.Head>
              <Table.Head>Type</Table.Head>
              <Table.Head>Category</Table.Head>
              <Table.Head>Record data</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {changes.map((c, i) => (
              <Table.Row key={i}>
                <Table.Cell>
                  <KindBadge kind={c.kind} />
                </Table.Cell>
                <Table.Cell>
                  <Text variant="mono">{c.name}</Text>
                </Table.Cell>
                <Table.Cell>
                  <Text variant="mono">{c.type}</Text>
                </Table.Cell>
                <Table.Cell>
                  <CategoryBadge category={c.category} />
                </Table.Cell>
                <Table.Cell>
                  <RDataCell change={c} />
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table>
      </div>
    </LayerCard>
  );
}

function RDataCell({ change }: { change: Change }) {
  if (change.kind === "modified") {
    return (
      <div className="stack" style={{ gap: "0.25rem" }}>
        <RDataLine label="old:" ttl={change.old_ttl} rdata={change.old_rdata} keyInfo={change.old_key} />
        <RDataLine label="new:" ttl={change.new_ttl} rdata={change.new_rdata} keyInfo={change.new_key} />
      </div>
    );
  }
  const removed = change.kind === "removed";
  return (
    <RDataLine
      ttl={removed ? change.old_ttl : change.new_ttl}
      rdata={removed ? change.old_rdata : change.new_rdata}
      keyInfo={removed ? change.old_key : change.new_key}
    />
  );
}

// RDataLine は RDATA 1 件分の表示。
// DNSKEY / CDNSKEY の注記は base64 の折り返しに埋もれないよう別行にする。
function RDataLine({
  label,
  ttl,
  rdata,
  keyInfo,
}: {
  label?: string;
  ttl?: number;
  rdata?: string;
  keyInfo?: KeyInfo;
}) {
  const annotation = formatKeyAnnotation(keyInfo);
  return (
    <div className="rdata">
      {label && (
        <Text variant="mono-secondary" as="span">
          {label}{" "}
        </Text>
      )}
      <Text variant="mono" as="span">
        {formatRData(ttl, rdata)}
      </Text>
      {annotation && (
        <div className="key-annotation">
          <Text variant="mono-secondary" as="span">
            {annotation}
          </Text>
        </div>
      )}
    </div>
  );
}

function formatRData(ttl?: number, rdata?: string): string {
  const parts: string[] = [];
  // TTL 0 は有効な値なので、フィールドが存在する限り表示する
  if (ttl !== undefined) parts.push(`TTL ${ttl}`);
  if (rdata) parts.push(rdata);
  return parts.join("  ") || "-";
}

// formatKeyAnnotation は DNSKEY / CDNSKEY の注記を組み立てる。
// 通知 (internal/dnssec の KeyInfo.Annotation) と同じ表記にする。
function formatKeyAnnotation(key?: KeyInfo): string {
  if (!key) return "";
  let s = `key tag = ${key.tag} (${key.role})`;
  if (key.size) s += `, size = ${key.size}`;
  return s;
}

export function DiffDetailPage({ variant = "zone" }: { variant?: DetailVariant }) {
  const { id } = useParams<{ id: string }>();
  const [entry, setEntry] = useState<DiffEntry | null>(null);
  const [error, setError] = useState<string | null>(null);
  // #DNSSEC / #glue のようなフラグメントがあればそのタブを初期表示する。
  const [tab, setTab] = useState(() => tabFromHash(window.location.hash));
  const [page, setPage] = useState(1);
  const tabsRef = useRef<HTMLDivElement>(null);
  const labels = detailLabels[variant];

  // エントリ (id) またはタブ種別 (variant) が変わったらタブとページを初期化する。
  // render 中の state 調整で行うことで、effect は初期化後の値で 1 回だけ走る。
  const [prevKey, setPrevKey] = useState({ id, variant });
  if (prevKey.id !== id || prevKey.variant !== variant) {
    setPrevKey({ id, variant });
    setEntry(null);
    setError(null);
    setTab(tabFromHash(window.location.hash));
    setPage(1);
  }

  // アドレスバーでのフラグメント編集やブラウザの戻る/進むに追随する。
  // タブ切替は History API の replaceState を使うため hashchange は発火しない。
  useEffect(() => {
    const categories = entry ? Object.keys(entry.summary.by_category) : null;
    const syncFromHash = () => {
      const raw = tabFromHash(window.location.hash);
      // データ取得済みなら、大文字小文字の違いや未知のカテゴリをこの場で解決する。
      // 同じタブに正規化される場合はページ位置を維持する。
      const next = categories ? resolveTab(raw, categories) : raw;
      if (next !== tab) {
        setTab(next);
        setPage(1);
      }
      // #all や #dnssec のようなフラグメントはこの時点で正規化する。
      replaceTabHash(next);
    };
    window.addEventListener("hashchange", syncFromHash);
    return () => window.removeEventListener("hashchange", syncFromHash);
  }, [entry, tab]);

  // タブ切替を URL のフラグメントに反映する。
  useEffect(() => {
    replaceTabHash(tab);
  }, [tab]);

  // 選択中のタブが横スクロール領域の外にある場合 (パーマリンクで直接開いた場合など)
  // は見える位置までスクロールする。
  useEffect(() => {
    if (entry === null) return;
    const list = tabsRef.current?.querySelector<HTMLElement>('[role="tablist"]');
    const active = list?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]');
    if (!list || !active) return;
    const centered = active.offsetLeft - (list.clientWidth - active.offsetWidth) / 2;
    list.scrollLeft = Math.max(0, Math.min(centered, list.scrollWidth - list.clientWidth));
  }, [entry, tab]);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    // ページ・タブ切替時は前の内容を表示し続け、応答が来たら差し替える
    // (都度ローダーを出すより体感が良い。順序乱れは cancelled で防ぐ)。
    detailFetchers[variant](id, {
      category: tab === ALL_TAB ? undefined : tab,
      page: page,
      perPage: DETAIL_PER_PAGE,
    })
      .then((resp) => {
        if (cancelled) return;
        // #dnssec のような大文字小文字違いのフラグメントは、この時点では
        // category=dnssec で 0 件になる。空の結果を一瞬描画しないよう、
        // カテゴリが確定したらタブを正規化して再取得に任せる。
        const resolved = resolveTab(tab, Object.keys(resp.summary.by_category));
        if (resolved !== tab) {
          setTab(resolved);
          setPage(1);
          return;
        }
        setEntry(resp);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      });
    return () => {
      cancelled = true;
    };
  }, [id, variant, tab, page]);

  if (error) {
    return (
      <div className="stack">
        <Breadcrumbs>
          <Breadcrumbs.Link href={labels.listPath}>{labels.listName}</Breadcrumbs.Link>
          <Breadcrumbs.Separator />
          <Breadcrumbs.Current>{id}</Breadcrumbs.Current>
        </Breadcrumbs>
        <Banner
          icon={<WarningCircleIcon weight="fill" />}
          variant="error"
          title="Failed to load diff"
          description={error}
        />
      </div>
    );
  }

  if (entry === null) {
    return (
      <div className="center">
        <Loader size="lg" />
      </div>
    );
  }

  const categories = Object.keys(entry.summary.by_category).sort();

  // ハッシュが未知のカテゴリを指している場合は「すべて」に戻す。
  // render 中の state 調整なので、無効なタブのまま画面に描画されない。
  const resolvedTab = resolveTab(tab, categories);
  if (resolvedTab !== tab) {
    setTab(resolvedTab);
    setPage(1);
  }

  return (
    <div className="stack">
      <Breadcrumbs>
        <Breadcrumbs.Link href={labels.listPath}>{labels.listName}</Breadcrumbs.Link>
        <Breadcrumbs.Separator />
        <Breadcrumbs.Current>{entry.new_serial}</Breadcrumbs.Current>
      </Breadcrumbs>

      <LayerCard>
        <LayerCard.Secondary className="row-between">
          <span>Detected {formatDetectedAt(entry.detected_at)}</span>
          <SummaryBadges summary={entry.summary} />
        </LayerCard.Secondary>
        <LayerCard.Primary>
          <div className="meta-grid">
            <div>
              <Text variant="secondary" size="sm">
                {labels.previous}
              </Text>
              <Text variant="mono">{entry.old_serial || "-"}</Text>
            </div>
            <div>
              <Text variant="secondary" size="sm">
                {labels.next}
              </Text>
              <Text variant="mono">{entry.new_serial}</Text>
            </div>
            <div>
              <Text variant="secondary" size="sm">
                Total changes
              </Text>
              <Text>{entry.summary.total}</Text>
            </div>
          </div>
        </LayerCard.Primary>
      </LayerCard>

      <div ref={tabsRef}>
        <Tabs
          variant="underline"
          listClassName="tabs-scroll"
          tabs={[
            { value: ALL_TAB, label: `All (${entry.summary.total})` },
            ...categories.map((cat) => ({
              value: cat,
              label: `${cat} (${entry.summary.by_category[cat]})`,
            })),
          ]}
          value={tab}
          onValueChange={(value) => {
            setTab(value);
            setPage(1);
          }}
        />
      </div>

      <ChangeTable changes={entry.changes} />

      {(entry.total_pages ?? 1) > 1 && (
        <Pagination
          page={page}
          setPage={setPage}
          perPage={entry.per_page ?? DETAIL_PER_PAGE}
          totalCount={entry.changes_total ?? entry.summary.total}
        >
          <Pagination.Info />
          <Pagination.Controls />
        </Pagination>
      )}
    </div>
  );
}
