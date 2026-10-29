import { Banner, Empty, LayerCard, Loader, Pagination, Table, Text } from "@cloudflare/kumo";
import { FileTextIcon, WarningCircleIcon } from "@phosphor-icons/react";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  fetchAnchorDiffs,
  fetchDiffs,
  fetchTLDDiffs,
  formatDetectedAt,
  type DiffListResponse,
} from "../api.ts";
import { SummaryBadges } from "../components/badges.tsx";

const PER_PAGE = 20;

export type ListVariant = "zone" | "anchors" | "tlds";

interface ListConfig {
  fetch: (page: number, perPage: number) => Promise<DiffListResponse>;
  detailPath: (id: string) => string;
  serialLabel: string;
  tableLabel: string;
  errorTitle: string;
  emptyTitle: string;
  emptyDescription: string;
}

const listConfigs: Record<ListVariant, ListConfig> = {
  zone: {
    fetch: fetchDiffs,
    detailPath: (id) => `/diffs/${id}`,
    serialLabel: "Serial",
    tableLabel: "Diffs table",
    errorTitle: "Failed to load diffs",
    emptyTitle: "No changes recorded yet",
    emptyDescription: "Root zone changes will appear here once they are detected.",
  },
  anchors: {
    fetch: fetchAnchorDiffs,
    detailPath: (id) => `/anchors/diffs/${id}`,
    serialLabel: "Trust anchor id",
    tableLabel: "Root anchor diffs table",
    errorTitle: "Failed to load root anchor diffs",
    emptyTitle: "No root anchor changes recorded yet",
    emptyDescription: "Root anchor changes will appear here once they are detected.",
  },
  tlds: {
    fetch: fetchTLDDiffs,
    detailPath: (id) => `/tlds/diffs/${id}`,
    serialLabel: "Version",
    tableLabel: "TLD list diffs table",
    errorTitle: "Failed to load TLD list diffs",
    emptyTitle: "No TLD list changes recorded yet",
    emptyDescription: "TLD list changes will appear here once they are detected.",
  },
};

export function DiffListPage({ variant = "zone" }: { variant?: ListVariant }) {
  const [page, setPage] = useState(1);
  const [data, setData] = useState<DiffListResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const config = listConfigs[variant];

  // ルート間でコンポーネントが再利用された場合に、前のタブの一覧を
  // 一瞬表示しないようページと取得結果をリセットする。
  const [prevVariant, setPrevVariant] = useState(variant);
  if (prevVariant !== variant) {
    setPrevVariant(variant);
    setPage(1);
    setData(null);
    setError(null);
  }

  useEffect(() => {
    let cancelled = false;
    setError(null);
    config
      .fetch(page, PER_PAGE)
      .then((resp) => {
        if (!cancelled) setData(resp);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      });
    return () => {
      cancelled = true;
    };
  }, [config, page]);

  if (error) {
    return (
      <Banner
        icon={<WarningCircleIcon weight="fill" />}
        variant="error"
        title={config.errorTitle}
        description={error}
      />
    );
  }

  if (data === null) {
    return (
      <div className="center">
        <Loader size="lg" />
      </div>
    );
  }

  if (data.total === 0) {
    return (
      <Empty
        icon={<FileTextIcon size={48} />}
        title={config.emptyTitle}
        description={config.emptyDescription}
      />
    );
  }

  return (
    <div className="stack">
      <LayerCard className="p-0">
        <div className="table-scroll" role="region" aria-label={config.tableLabel} tabIndex={0}>
          <Table>
            <Table.Header>
              <Table.Row>
                <Table.Head>Detected</Table.Head>
                <Table.Head>{config.serialLabel}</Table.Head>
                <Table.Head>Changes</Table.Head>
                <Table.Head>Categories</Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {data.diffs.map((d) => (
                <Table.Row
                  key={d.id}
                  className="clickable-row"
                  onClick={() => navigate(config.detailPath(d.id))}
                >
                  <Table.Cell>{formatDetectedAt(d.detected_at)}</Table.Cell>
                  <Table.Cell>
                    <Text variant="mono">{d.new_serial}</Text>
                  </Table.Cell>
                  <Table.Cell>
                    <span className="row">
                      {d.summary.total}
                      <SummaryBadges summary={d.summary} />
                    </span>
                  </Table.Cell>
                  <Table.Cell>
                    <Text variant="secondary" size="sm" as="span">
                      {Object.entries(d.summary.by_category)
                        .map(([cat, count]) => `${cat}: ${count}`)
                        .join(", ")}
                    </Text>
                  </Table.Cell>
                </Table.Row>
              ))}
            </Table.Body>
          </Table>
        </div>
      </LayerCard>
      {data.total > PER_PAGE && (
        <Pagination page={page} setPage={setPage} perPage={PER_PAGE} totalCount={data.total}>
          <Pagination.Info />
          <Pagination.Controls />
        </Pagination>
      )}
    </div>
  );
}
