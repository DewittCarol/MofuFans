'use client';

import { Upload, X } from 'lucide-react';

export function VoyageImageEditor({ images, limit, label, disabled, onChange, onUpload }: {
  images: string[];
  limit: number;
  label: string;
  disabled: boolean;
  onChange: (images: string[]) => void;
  onUpload: (files: File[]) => void;
}) {
  return <div className="space-y-3">
    <div className="flex flex-wrap gap-3">
      {images.map((url, index) => <div key={`${url}-${index}`} className="relative h-28 w-28">
        <img src={url} alt={`${label} ${index + 1}`} loading="lazy" className="h-full w-full rounded-lg bg-secondary object-contain" />
        <button type="button" disabled={disabled} aria-label={`移除${label} ${index + 1}`}
          className="absolute right-0 top-0 flex h-9 w-9 items-center justify-center rounded-bl-lg rounded-tr-lg bg-card text-muted-foreground hover:text-destructive focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-50"
          onClick={() => onChange(images.filter((_, i) => i !== index))}><X size={16} /></button>
      </div>)}
      {images.length < limit && <label className={`flex h-28 w-28 flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-border text-sm text-primary focus-within:outline-2 focus-within:outline-ring ${disabled ? 'opacity-50' : 'cursor-pointer hover:bg-accent'}`}>
        <Upload size={20} />上传图片
        <input type="file" multiple disabled={disabled} accept="image/png,image/jpeg,image/webp" className="sr-only" aria-label={`上传${label}`}
          onChange={event => { const files = Array.from(event.target.files || []); event.target.value = ''; onUpload(files); }} />
      </label>}
    </div>
    <p className="text-xs leading-relaxed text-muted-foreground">{images.length} / {limit} 张 · 支持 JPG、PNG、WebP，每张不超过 12 MB</p>
  </div>;
}
