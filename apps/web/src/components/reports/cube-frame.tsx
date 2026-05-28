import * as React from 'react';
import { buildCubeFrameUrl, type DashboardSpec } from '@/lib/reports';
import { cn } from '@/lib/utils';

export interface CubeFrameProps {
  token: string;
  dashboard: DashboardSpec;
  className?: string;
  // Override for tests.
  minHeight?: number;
}

// Frame messages we accept from the embed page. Anything else is
// silently ignored — the iframe runs untrusted JS (Cube + chart libs)
// and we don't want to invite postMessage abuse.
type EmbedMessage =
  | { type: 'cube-embed:resize'; height: number }
  | { type: 'cube-embed:ready' };

function isEmbedMessage(value: unknown): value is EmbedMessage {
  if (!value || typeof value !== 'object') return false;
  const v = value as { type?: unknown };
  return (
    v.type === 'cube-embed:resize' || v.type === 'cube-embed:ready'
  );
}

export function CubeFrame({
  token,
  dashboard,
  className,
  minHeight = 480,
}: CubeFrameProps) {
  const src = React.useMemo(
    () => buildCubeFrameUrl(token, dashboard.id),
    [token, dashboard.id],
  );

  const [height, setHeight] = React.useState<number>(minHeight);
  const iframeRef = React.useRef<HTMLIFrameElement>(null);

  React.useEffect(() => {
    function onMessage(event: MessageEvent) {
      // Only trust messages from our own iframe — origin check is
      // deliberately loose here (Cube embed may be on a sibling
      // subdomain), but we require the message to come from the frame
      // we own.
      if (event.source !== iframeRef.current?.contentWindow) return;
      if (!isEmbedMessage(event.data)) return;
      if (event.data.type === 'cube-embed:resize') {
        const next = Math.max(minHeight, Math.floor(event.data.height));
        setHeight(next);
      }
    }
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, [minHeight]);

  return (
    <iframe
      ref={iframeRef}
      title={`Cube dashboard — ${dashboard.title}`}
      src={src}
      // sandbox allows the embedded page to run JS and talk back via
      // postMessage but blocks form submission, top-level navigation,
      // and same-origin escapes. The Cube REST call is cross-origin
      // so it does not need allow-same-origin.
      sandbox="allow-scripts allow-popups"
      className={cn(
        'w-full rounded-md border bg-card transition-[height] duration-150',
        className,
      )}
      style={{ height }}
    />
  );
}
