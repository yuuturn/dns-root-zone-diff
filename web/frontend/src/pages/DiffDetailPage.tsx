import { Banner, Breadcrumbs, Empty, LayerCard, Loader, Table, Tabs, Text } from "@cloudflare/kumo";
import { WarningCircleIcon } from "@phosphor-icons/react";
import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import {
  fetchAnchorDiff,
  fetchDiff,
  formatDetectedAt,
  type Change,
  type DiffEntry,
} from "../api.ts";
import { CategoryBadge, KindBadge, SummaryBadges } from "../components/badges.tsx";

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
        <div className="rdata">
          <Text variant="mono-secondary" as="span">
            old:{" "}
          </Text>
          <Text variant="mono" as="span">
            {formatRData(change.old_ttl, change.old_rdata)}
          </Text>
        </div>
        <div className="rdata">
          <Text variant="mono-secondary" as="span">
            new:{" "}
          </Text>
          <Text variant="mono" as="span">
            {formatRData(change.new_ttl, change.new_rdata)}
          </Text>
        </div>
      </div>
    );
  }
  const ttl = change.kind === "removed" ? change.old_ttl : change.new_ttl;
  const rdata = change.kind === "removed" ? change.old_rdata : change.new_rdata;
  return (
    <div className="rdata">
      <Text variant="mono" as="span">
        {formatRData(ttl, rdata)}
      </Text>
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

export function DiffDetailPage({ variant = "zone" }: { variant?: "zone" | "anchors" }) {
  const { id } = useParams<{ id: string }>();
  const [entry, setEntry] = useState<DiffEntry | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState("all");
  const isAnchors = variant === "anchors";

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    setEntry(null);
    setError(null);
    const fetch = isAnchors ? fetchAnchorDiff : fetchDiff;
    fetch(id)
      .then((resp) => {
        if (!cancelled) setEntry(resp);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      });
    return () => {
      cancelled = true;
    };
  }, [id, isAnchors]);

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
  const visibleChanges =
    tab === "all" ? entry.changes : entry.changes.filter((c) => c.category === tab);

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
        onValueChange={setTab}
      />

      <ChangeTable changes={visibleChanges} />
    </div>
  );
}
