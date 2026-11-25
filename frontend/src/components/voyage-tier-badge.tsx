import { voyageTiers, type VoyageTier } from '@/lib/voyage';

export function VoyageTierBadge({ tier }: { tier: VoyageTier }) {
  const item = voyageTiers.find(item => item.id === tier)!;
  return <span className="inline-flex shrink-0 items-center gap-1.5 text-sm font-medium leading-5">
    <img src={item.icon} width={18} height={18} alt="" className="h-[18px] w-[18px] shrink-0 object-contain" />
    {item.name}
  </span>;
}
