export function attachmentUrls(images: string): string[] {
  return images.split(';').map(value => value.trim()).filter(Boolean).flatMap(value => {
    if (/^https?:\/\//i.test(value)) return [value];
    if (/^[a-z][a-z\d+.-]*:/i.test(value) || value.startsWith('//')) return [];
    if (value.startsWith('/')) return [value];
    // Uploads stored by JoiAsk have historically omitted the leading slash.
    return ['/' + value.replace(/^(?:\.\/)+/, '')];
  });
}
