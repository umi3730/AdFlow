import Image from 'next/image';
import { displayCreativeURL } from '@/lib/demo-creatives';

export function CreativeImage({
  src,
  alt,
  size = 80,
}: {
  src: string;
  alt: string;
  size?: 64 | 80 | 160;
}) {
  return (
    <Image
      src={displayCreativeURL(src)}
      alt={alt}
      width={size}
      height={size}
      unoptimized
      draggable={false}
      style={{
        width: size,
        height: size,
        maxWidth: '100%',
        objectFit: 'contain',
        imageRendering: 'pixelated',
        flexShrink: 0,
      }}
      className="pointer-events-none block"
    />
  );
}
