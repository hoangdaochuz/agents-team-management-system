import type { ReactNode } from "react";
import { EmptyState } from "./EmptyState";

/** Standardize the loading / error / empty / data rendering for a React Query result. */
export function AsyncBoundary<T>({
  isLoading,
  isError,
  error,
  data,
  isEmpty,
  emptyTitle = "Nothing here yet",
  emptyHint,
  children,
}: {
  isLoading: boolean;
  isError: boolean;
  error?: unknown;
  data: T | undefined;
  isEmpty?: (d: T) => boolean;
  emptyTitle?: ReactNode;
  emptyHint?: ReactNode;
  children: (data: T) => ReactNode;
}) {
  if (isLoading) {
    return (
      <div className="muted" style={{ padding: "var(--space-6)" }}>
        Loading…
      </div>
    );
  }
  if (isError) {
    const msg = error instanceof Error ? error.message : "Unable to load";
    return (
      <EmptyState
        icon="alert"
        title="Couldn't reach the backend"
        hint={`${msg}. Endpoints aren't implemented yet — the UI still renders against the declared contract.`}
      />
    );
  }
  if (data === undefined) {
    return <EmptyState title={emptyTitle} hint={emptyHint} />;
  }
  if (isEmpty && isEmpty(data)) {
    return <EmptyState title={emptyTitle} hint={emptyHint} />;
  }
  return <>{children(data)}</>;
}
