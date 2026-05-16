// Skeleton placeholders shown while data loads. Sized to match the real
// components so the layout doesn't jump when data arrives (spec § 8: "skeleton
// cards, not raw spinners").

// SkeletonCard mirrors MatchCard's dimensions.
export function SkeletonCard() {
  return (
    <div className="overflow-hidden rounded-lg border border-zinc-800">
      <div className="flex items-stretch">
        <div className="flex-1 px-4 py-3">
          <div className="h-4 w-32 animate-pulse rounded bg-zinc-800" />
        </div>
        <div className="flex shrink-0 items-center px-3">
          <div className="h-4 w-6 animate-pulse rounded bg-zinc-800" />
        </div>
        <div className="flex flex-1 justify-end px-4 py-3">
          <div className="h-4 w-32 animate-pulse rounded bg-zinc-800" />
        </div>
      </div>
    </div>
  );
}

// SkeletonSection mirrors a RoundSection: a header bar plus a few cards.
export function SkeletonSection() {
  return (
    <section className="space-y-3">
      <div className="h-4 w-40 animate-pulse rounded bg-zinc-800" />
      <div className="space-y-2">
        <SkeletonCard />
        <SkeletonCard />
        <SkeletonCard />
      </div>
    </section>
  );
}

// SkeletonRow mirrors a LeaderboardRow.
export function SkeletonRow() {
  return (
    <li>
      <div className="flex items-center justify-between rounded-md border border-zinc-800 bg-zinc-900 px-4 py-3">
        <span className="flex items-center gap-3">
          <div className="h-4 w-4 animate-pulse rounded bg-zinc-800" />
          <div className="h-4 w-32 animate-pulse rounded bg-zinc-800" />
        </span>
        <div className="h-4 w-6 animate-pulse rounded bg-zinc-800" />
      </div>
    </li>
  );
}
