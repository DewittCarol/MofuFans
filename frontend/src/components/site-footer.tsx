"use client";
import { useEmojiCatalog } from "@/lib/emoji-catalog";
import { policeLink } from "../../../shared/footer";
import { safeAsset } from "../../../shared/theme";

export function SiteFooter() {
  const { footer } = useEmojiCatalog();
  if (!footer.footer_icp_number && !footer.footer_police_number) return null;
  return <footer aria-label="网站备案信息" className="relative mx-auto flex w-full max-w-5xl flex-wrap items-center justify-center gap-x-5 gap-y-2 px-5 pb-8 pt-6 text-center text-xs leading-relaxed text-muted-foreground">
    {footer.footer_icp_number && <a className="break-words underline-offset-4 hover:underline focus-visible:underline" href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">{footer.footer_icp_number}</a>}
    {footer.footer_police_number && <a className="inline-flex items-center gap-1.5 break-words underline-offset-4 hover:underline focus-visible:underline" href={policeLink(footer.footer_police_url)} target="_blank" rel="noopener noreferrer">{footer.footer_police_icon && safeAsset(footer.footer_police_icon) && <img src={footer.footer_police_icon} alt="" className="h-4 w-4 shrink-0 object-contain" />}{footer.footer_police_number}</a>}
  </footer>;
}
