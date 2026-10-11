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
import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import {
  fetchAnchorDiff,
  fetchDiff,
  formatDetectedAt,
  type Change,
  type DiffEntry,
  type KeyInfo,
} from "../api.ts";
import { CategoryBadge, KindBadge, SummaryBadges } from "../components/badges.tsx";

// 詳細 API の既定ページング件数 (サーバ側 defaultDetailPerPage と一致させる)
const DETAIL_PER_PAGE = 100;

function ChangeTable({ changes }: { changes: Change[] }) {
  if (changes.length === 0) {
    return <Empty size="sm" title="No changes in this category" />;
  }
  return (
    <LayerCard className="p-0">
      <Table>
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

export function DiffDetailPage({ variant = "zone" }: { variant?: "zone" | "anchors" }) {
  const { id } = useParams<{ id: string }>();
  const [entry, setEntry] = useState<DiffEntry | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState("all");
  const [page, setPage] = useState(1);
  const isAnchors = variant === "anchors";

  // エントリ (id) が変わったらタブとページを初期化する。
  // render 中の state 調整で行うことで、effect は初期化後の値で 1 回だけ走る。
  const [prevId, setPrevId] = useState(id);
  if (prevId !== id) {
    setPrevId(id);
    setEntry(null);
    setError(null);
    setTab("all");
    setPage(1);
  }

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    // ページ・タブ切替時は前の内容を表示し続け、応答が来たら差し替える
    // (都度ローダーを出すより体感が良い。順序乱れは cancelled で防ぐ)。
    const fetch = isAnchors ? fetchAnchorDiff : fetchDiff;
    fetch(id, {
      category: tab === "all" ? undefined : tab,
      page: page,
      perPage: DETAIL_PER_PAGE,
    })
      .then((resp) => {
        if (!cancelled) setEntry(resp);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      });
    return () => {
      cancelled = true;
    };
  }, [id, isAnchors, tab, page]);

  if (error) {
    return (
      <div className="stack">
        <Breadcrumbs>
          <Breadcrumbs.Link href={isAnchors ? "/anchors" : "/"}>
            {isAnchors ? "Anchors" : "Diffs"}
          </Breadcrumbs.Link>
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

  return (
    <div className="stack">
      <Breadcrumbs>
        <Breadcrumbs.Link href={isAnchors ? "/anchors" : "/"}>
          {isAnchors ? "Anchors" : "Diffs"}
        </Breadcrumbs.Link>
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
                {isAnchors ? "Previous anchor id" : "Previous serial"}
              </Text>
              <Text variant="mono">{entry.old_serial || "-"}</Text>
            </div>
            <div>
              <Text variant="secondary" size="sm">
                {isAnchors ? "New anchor id" : "New serial"}
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

      <Tabs
        variant="underline"
        tabs={[
          { value: "all", label: `All (${entry.summary.total})` },
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
