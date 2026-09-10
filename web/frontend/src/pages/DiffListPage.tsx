import { Banner, Empty, LayerCard, Loader, Pagination, Table, Text } from "@cloudflare/kumo";
import { FileTextIcon, WarningCircleIcon } from "@phosphor-icons/react";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { fetchDiffs, formatDetectedAt, type DiffListResponse } from "../api.ts";
import { SummaryBadges } from "../components/badges.tsx";

const PER_PAGE = 20;

export function DiffListPage() {
  const [page, setPage] = useState(1);
  const [data, setData] = useState<DiffListResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    let cancelled = false;
    setError(null);
    fetchDiffs(page, PER_PAGE)
      .then((resp) => {
        if (!cancelled) setData(resp);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      });
    return () => {
      cancelled = true;
    };
  }, [page]);

  if (error) {
    return (
      <Banner
        icon={<WarningCircleIcon weight="fill" />}
        variant="error"
        title="Failed to load diffs"
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
        title="No changes recorded yet"
        description="Root zone changes will appear here once they are detected."
      />
    );
  }

  return (
    <div className="stack">
      <LayerCard className="p-0">
        <Table>
          <Table.Header>
            <Table.Row>
              <Table.Head>Detected</Table.Head>
              <Table.Head>Serial</Table.Head>
              <Table.Head>Changes</Table.Head>
              <Table.Head>Categories</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {data.diffs.map((d) => (
              <Table.Row
                key={d.id}
                className="clickable-row"
                onClick={() => navigate(`/diffs/${d.id}`)}
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
