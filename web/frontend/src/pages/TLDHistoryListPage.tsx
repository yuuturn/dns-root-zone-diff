import {
  Banner,
  Button,
  Empty,
  Input,
  LayerCard,
  Loader,
  Pagination,
  Table,
  Text,
} from "@cloudflare/kumo";
import { FileTextIcon, MagnifyingGlassIcon, WarningCircleIcon } from "@phosphor-icons/react";
import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  fetchTLDHistory,
  formatDetectedAt,
  type TLDHistoryListResponse,
} from "../api.ts";

const PER_PAGE = 50;
const TLD_PATTERN = /^[a-z0-9-]{1,63}$/;

// normalizeTLD は検索入力を API / URL と同じ形式 (小文字・末尾ドット無し) にする。
function normalizeTLD(value: string): string {
  return value.trim().replace(/^\.+|\.+$/g, "").toLowerCase();
}

export function TLDHistoryListPage() {
  const [page, setPage] = useState(1);
  const [data, setData] = useState<TLDHistoryListResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [searchError, setSearchError] = useState<string | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    let cancelled = false;
    setError(null);
    fetchTLDHistory(page, PER_PAGE)
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

  const openTLD = () => {
    const tld = normalizeTLD(query);
    if (!TLD_PATTERN.test(tld)) {
      setSearchError("Enter a TLD such as com, example, or xn--p1ai.");
      return;
    }
    setSearchError(null);
    navigate(`/tld-history/${encodeURIComponent(tld)}`);
  };

  return (
    <div className="stack">
      <LayerCard>
        <LayerCard.Primary>
          <form
            className="tld-search"
            onSubmit={(event) => {
              event.preventDefault();
              openTLD();
            }}
          >
            <div className="tld-search-field">
              <Input
                label="Search TLD"
                placeholder="example"
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  setSearchError(null);
                }}
                autoComplete="off"
                spellCheck={false}
                className="tld-search-input"
              />
            </div>
            <Button type="submit" variant="primary" icon={MagnifyingGlassIcon}>
              Open
            </Button>
          </form>
          <p className="tld-search-hint">
            Root zone changes are grouped by TLD. RRSIG re-signing is excluded.
          </p>
          {searchError && (
            <Banner
              icon={<WarningCircleIcon weight="fill" />}
              variant="error"
              title="Invalid TLD"
              description={searchError}
            />
          )}
        </LayerCard.Primary>
      </LayerCard>

      {error ? (
        <Banner
          icon={<WarningCircleIcon weight="fill" />}
          variant="error"
          title="Failed to load TLD history"
          description={error}
        />
      ) : data === null ? (
        <div className="center">
          <Loader size="lg" />
        </div>
      ) : data.total === 0 ? (
        <Empty
          icon={<FileTextIcon size={48} />}
          title="No TLD-level changes recorded yet"
          description="Root zone changes will appear here once they are detected. RRSIG re-signing is excluded."
        />
      ) : (
        <>
          <LayerCard className="p-0">
            <div className="table-scroll" role="region" aria-label="TLD history table" tabIndex={0}>
              <Table>
                <Table.Header>
                  <Table.Row>
                    <Table.Head>TLD</Table.Head>
                    <Table.Head>Last change</Table.Head>
                    <Table.Head>Events</Table.Head>
                    <Table.Head>Changes</Table.Head>
                    <Table.Head>Categories</Table.Head>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {data.tlds.map((item) => {
                    const path = `/tld-history/${encodeURIComponent(item.tld)}`;
                    return (
                      <Table.Row
                        key={item.tld}
                        className="clickable-row"
                        onClick={() => navigate(path)}
                      >
                        <Table.Cell>
                          <Link
                            className="tld-link"
                            to={path}
                            onClick={(event) => event.stopPropagation()}
                          >
                            <Text variant="mono">{item.tld}.</Text>
                          </Link>
                        </Table.Cell>
                        <Table.Cell>{formatDetectedAt(item.last_detected_at)}</Table.Cell>
                        <Table.Cell>{item.events}</Table.Cell>
                        <Table.Cell>{item.changes}</Table.Cell>
                        <Table.Cell>
                          <Text variant="secondary" size="sm" as="span">
                            {Object.entries(item.by_category)
                              .map(([cat, count]) => `${cat}: ${count}`)
                              .join(", ")}
                          </Text>
                        </Table.Cell>
                      </Table.Row>
                    );
                  })}
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
        </>
      )}
    </div>
  );
}
