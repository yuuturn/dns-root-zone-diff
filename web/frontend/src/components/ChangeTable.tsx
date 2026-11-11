import { Empty, LayerCard, Table, Text } from "@cloudflare/kumo";
import type { Change, KeyInfo } from "../api.ts";
import { CategoryBadge, KindBadge } from "./badges.tsx";

// ChangeTable は diff の変更レコード一覧。Root Zone の diff 詳細と
// TLD History のタイムラインで共通利用する。
export function ChangeTable({
  changes,
  emptyTitle = "No changes in this category",
}: {
  changes: Change[];
  emptyTitle?: string;
}) {
  if (changes.length === 0) {
    return <Empty size="sm" title={emptyTitle} />;
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
