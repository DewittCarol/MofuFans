export function galleryImageURL(src: string, variant: 'small' | 'thumb' | 'preview') {
  if (!/^\/api\/gallery\/assets\/[a-f0-9]{32}\.(jpg|png|webp|gif)$/.test(src)) return src;
  return `${src}?variant=${variant}-v1`;
}
