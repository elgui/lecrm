import * as React from 'react';
import { Download } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { ApiError } from '@/lib/api';

interface ExportButtonProps {
  /** Entity collection to export: contacts | companies | deals. */
  resource: 'contacts' | 'companies' | 'deals';
  label?: string;
}

// ExportButton downloads the workspace's CSV for a resource. It fetches the
// streamed file as a blob (so the session cookie rides along, unlike a bare
// anchor with a cross-origin concern) and triggers a client-side save using
// the server's Content-Disposition filename when present.
export function ExportButton({ resource, label = 'Exporter CSV' }: ExportButtonProps) {
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  const onClick = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await fetch(`/v1/${resource}/export?format=csv`, {
        headers: { Accept: 'text/csv' },
      });
      if (!res.ok) {
        throw new ApiError(res.status, await res.text());
      }
      const blob = await res.blob();
      const disposition = res.headers.get('Content-Disposition') ?? '';
      const match = /filename="?([^"]+)"?/.exec(disposition);
      const filename = match?.[1] ?? `${resource}.csv`;

      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Échec de l’export');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col items-end gap-1">
      <Button variant="outline" size="sm" onClick={onClick} disabled={busy}>
        <Download className="mr-2 h-4 w-4" />
        {busy ? 'Exportation…' : label}
      </Button>
      {error && <span className="text-xs text-destructive">{error}</span>}
    </div>
  );
}
