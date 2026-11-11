import {
  Banner,
  Breadcrumbs,
  Empty,
  LayerCard,
  Loader,
  Pagination,
  Text,
} from "@cloudflare/kumo";
import { FileTextIcon, WarningCircleIcon } from "@phosphor-icons/react";
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  fetchTLDHistoryDetail,
  formatDetectedAt,
  type TLDHistoryDetailResponse,
} from "../api.ts";
import { SummaryBadges } from "../components/badges.tsx";
import { ChangeTable } from "../components/ChangeTable.tsx";

const DETAIL_PER_PAGE = 20;

function categorySummary(categories: Record<string, number>): string {
  const entries = Object.entries(categories);
  if (entries.length === 0) return "-";
  return entries.map(([cat, count]) => `${cat}: ${count}`).join(", ");
}

export function TLDHistoryDetailPage() {
  const { tld } = useParams<{ tld: string }>();
  const [page, setPage] = useState(1);
  const [data, setData] = useState<TLDHistoryDetailResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  // 別の TLD へ移動したときに前の TLD の履歴を一瞬表示しない。
  const [prevTLD, setPrevTLD] = useState(tld);
  if (prevTLD !== tld) {
    setPrevTLD(tld);
    setPage(1);
    setData(null);
    setError(null);
  }

  useEffect(() => {
    if (!tld) return;
    let cancelled = false;
    setError(null);
    fetchTLDHistoryDetail(tld, page, DETAIL_PER_PAGE)
      .then((resp) => {
        if (!cancelled) setData(resp);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      });
    return () => {
      cancelled = true;
    };
  }, [tld, page]);

  if (error) {
    return (
      <div className="stack">
        <Breadcrumbs>
          <Breadcrumbs.Link href="/tld-history">TLD History</Breadcrumbs.Link>
          <Breadcrumbs.Separator />
          <Breadcrumbs.Current>{tld}</Breadcrumbs.Current>
        </Breadcrumbs>
        <Banner
          icon={<WarningCircleIcon weight="fill" />}
          variant="error"
          title="Failed to load TLD history"
          description={error}
        />
      </div>
    );
  }

  if (data === null) {
    return (
      <div className="center">
        <Loader size="lg" />
      </div>
    );
  }

  return (
    <div className="stack">
      <Breadcrumbs>
        <Breadcrumbs.Link href="/tld-history">TLD History</Breadcrumbs.Link>
        <Breadcrumbs.Separator />
        <Breadcrumbs.Current>{data.tld}.</Breadcrumbs.Current>
      </Breadcrumbs>

      <LayerCard>
        <LayerCard.Secondary className="row-between">
          <Text variant="heading3" as="h2">
            {data.tld}.
          </Text>
          <Text variant="secondary" size="sm" as="span">
            {data.stats.events} events · {data.stats.changes} changes
          </Text>
        </LayerCard.Secondary>
        <LayerCard.Primary>
          <div className="meta-grid">
            <div>
              <Text variant="secondary" size="sm">
                First change
              </Text>
              <Text>{formatDetectedAt(data.stats.first_detected_at) || "-"}</Text>
            </div>
            <div>
              <Text variant="secondary" size="sm">
                Last change
              </Text>
              <Text>{formatDetectedAt(data.stats.last_detected_at) || "-"}</Text>
            </div>
            <div>
              <Text variant="secondary" size="sm">
                Categories
              </Text>
              <Text variant="secondary" size="sm">
                {categorySummary(data.stats.by_category)}
              </Text>
            </div>
          </div>
        </LayerCard.Primary>
      </LayerCard>

      {data.events.length === 0 ? (
        <Empty
          icon={<FileTextIcon size={48} />}
          title={`No changes recorded for ${data.tld}.`}
          description="This TLD has no root zone changes in the stored history. RRSIG re-signing is excluded."
        />
      ) : (
        <>
          {data.events.map((event) => (
            <section className="timeline-event" key={event.id}>
              <LayerCard>
                <LayerCard.Secondary className="row-between">
                  <span>Detected {formatDetectedAt(event.detected_at)}</span>
                  <SummaryBadges summary={event.summary} />
                </LayerCard.Secondary>
                <LayerCard.Primary>
                  <div className="event-meta">
                    <Text variant="secondary" size="sm" as="span">
                      Serial
                    </Text>
                    <Text variant="mono" as="span">
                      {event.old_serial || "-"} → {event.new_serial}
                    </Text>
                    <Link
                      className="event-link"
                      to={`/diffs/${encodeURIComponent(event.id)}`}
                    >
                      View full diff →
                    </Link>
                  </div>
                </LayerCard.Primary>
              </LayerCard>
              <ChangeTable changes={event.changes} />
            </section>
          ))}
          {data.total > DETAIL_PER_PAGE && (
            <Pagination
              page={page}
              setPage={setPage}
              perPage={DETAIL_PER_PAGE}
              totalCount={data.total}
            >
              <Pagination.Info />
              <Pagination.Controls />
            </Pagination>
          )}
        </>
      )}
    </div>
  );
}
