import Image from 'next/image';

// Theme-only artwork. Deliberately separate from selectable advertising assets.
export function MygoArtwork({ className = '' }: { className?: string }) {
  return (
    <figure
      className={`overflow-hidden rounded-2xl border border-white/30 bg-white shadow-sm ${className}`}
    >
      <Image
        src="/theme-assets/mygo/five-shiramori.jpg"
        alt="白森さわ绘制的 MyGO 五人同人插画：高松灯、千早爱音、要乐奈、长崎素世和椎名立希"
        width={900}
        height={1200}
        unoptimized
        draggable={false}
        style={{
          width: '100%',
          height: 'clamp(180px, 22vw, 240px)',
          objectFit: 'contain',
        }}
        className="block"
      />
      <figcaption className="flex flex-wrap items-center justify-between gap-1 bg-white px-3 py-2 text-xs text-slate-600">
        <span>MyGO!!!!! · 五人同人图</span>
        <a
          href="https://www.pixiv.net/artworks/107693545"
          target="_blank"
          rel="noopener noreferrer"
          className="underline underline-offset-4 hover:text-primary"
        >
          白森さわ ↗
        </a>
      </figcaption>
    </figure>
  );
}
