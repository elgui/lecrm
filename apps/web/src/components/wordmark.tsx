import { cn } from '@/lib/utils';

/**
 * leCRM brand lockup: a crafted relationship-node mark (two linked nodes — the
 * CRM idea) beside a confident "leCRM" wordmark. Replaces the earlier
 * auto-generated-looking blue square + bare letter.
 */
export function Wordmark({ className }: { className?: string }) {
  return (
    <span className={cn('flex items-center gap-2.5', className)}>
      <svg
        width="28"
        height="28"
        viewBox="0 0 28 28"
        aria-hidden
        className="shrink-0 text-primary"
      >
        <rect width="28" height="28" rx="8" fill="currentColor" />
        {/* Two nodes joined by a link — relationships, the heart of a CRM. */}
        <path
          d="M11.4 11.4 16.6 16.6"
          stroke="#fff"
          strokeWidth="1.9"
          strokeLinecap="round"
        />
        <circle cx="9.8" cy="9.8" r="2.7" fill="#fff" />
        <circle cx="18.2" cy="18.2" r="2.7" fill="#fff" />
      </svg>
      <span className="text-[18px] font-bold leading-none tracking-tight text-foreground">
        le<span className="text-primary">CRM</span>
      </span>
    </span>
  );
}
