import { Badge } from "@cloudflare/kumo";
import type { Change, Summary } from "../api.ts";

const kindVariant = {
  added: "green",
  removed: "red",
  modified: "blue",
} as const;

export function KindBadge({ kind }: { kind: Change["kind"] }) {
  return <Badge variant={kindVariant[kind] ?? "neutral"}>{kind}</Badge>;
}

const categoryVariant: Record<string, "purple" | "teal" | "neutral"> = {
  delegation: "purple",
  DNSSEC: "teal",
};

export function CategoryBadge({ category }: { category: string }) {
  return <Badge variant={categoryVariant[category] ?? "neutral"}>{category}</Badge>;
}

export function SummaryBadges({ summary }: { summary: Summary }) {
  return (
    <span className="row">
      {summary.added > 0 && <Badge variant="green">+{summary.added}</Badge>}
      {summary.removed > 0 && <Badge variant="red">-{summary.removed}</Badge>}
      {summary.modified > 0 && <Badge variant="blue">~{summary.modified}</Badge>}
    </span>
  );
}
