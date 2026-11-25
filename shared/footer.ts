export type Footer = {
  footer_icp_number: string;
  footer_police_number: string;
  footer_police_url: string;
  footer_police_icon: string;
};
export const emptyFooter: Footer = { footer_icp_number: "", footer_police_number: "", footer_police_url: "", footer_police_icon: "" };
export function readFooter(value: Record<string, unknown>): Footer {
  return Object.fromEntries(Object.keys(emptyFooter).map(key => [key, typeof value[key] === "string" ? value[key] : ""])) as Footer;
}
export function policeLink(value: string): string | undefined {
  try {
    const url = new URL(value);
    if (url.protocol === "https:" && !url.username && !url.password && !url.port && ["beian.mps.gov.cn", "beian.gov.cn", "www.beian.gov.cn"].includes(url.hostname)) return url.href;
  } catch { /* An empty or invalid link is displayed as plain text. */ }
  return undefined;
}
