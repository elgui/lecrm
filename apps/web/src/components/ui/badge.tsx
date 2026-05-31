import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@/lib/utils';

const badgeVariants = cva(
  'inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2',
  {
    variants: {
      variant: {
        default:
          'border-transparent bg-blue-50 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300',
        secondary:
          'border-transparent bg-slate-100 text-slate-600 dark:bg-slate-500/20 dark:text-slate-300',
        success:
          'border-transparent bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
        warning:
          'border-transparent bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
        destructive:
          'border-transparent bg-rose-50 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300',
        outline: 'border-border text-muted-foreground',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}

export { Badge, badgeVariants };
