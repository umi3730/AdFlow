'use client';

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type RefObject,
} from 'react';
import { Check, GripVertical } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { CreativeImage } from '@/components/creative-image';
import { demoCreatives } from '@/lib/demo-creatives';
import {
  acceptsCreativeDrag,
  CREATIVE_DRAG_TYPE,
  droppedCreativeID,
} from '@/lib/creative-drag';

type PickerProps = {
  value: string;
  onChange: (id: string) => void;
  disabled?: boolean;
};

export function CreativeDropZone({
  value,
  onChange,
  disabled = false,
  onBrowse,
}: PickerProps & { onBrowse: () => void }) {
  const selected = demoCreatives.find((asset) => asset.id === value);
  const [hovered, setHovered] = useState(false);
  const [message, setMessage] = useState('');
  const depth = useRef(0);
  const endDrag = useCallback(() => {
    depth.current = 0;
    setHovered(false);
  }, []);
  useEffect(() => {
    window.addEventListener('dragend', endDrag);
    return () => window.removeEventListener('dragend', endDrag);
  }, [endDrag]);
  function select(id: string) {
    if (disabled) return;
    const asset = demoCreatives.find((item) => item.id === id);
    if (!asset) return;
    onChange(id);
    setMessage('已选中，点击下方按钮确认添加。');
  }
  return (
    <div className="space-y-2" data-testid="creative-drop-zone">
      <button
        type="button"
        aria-label="左侧已选素材接收区"
        aria-disabled={disabled}
        tabIndex={disabled ? -1 : 0}
        onClick={() => {
          if (!disabled) onBrowse();
        }}
        onDragEnter={(event) => {
          event.preventDefault();
          if (disabled) return;
          if (!acceptsCreativeDrag(event.dataTransfer)) {
            setMessage('仅支持右侧素材库中的小人。');
            return;
          }
          depth.current += 1;
          setHovered(true);
        }}
        onDragOver={(event) => {
          event.preventDefault();
          event.dataTransfer.dropEffect =
            !disabled && acceptsCreativeDrag(event.dataTransfer)
              ? 'copy'
              : 'none';
        }}
        onDragLeave={() => {
          depth.current = Math.max(0, depth.current - 1);
          if (depth.current === 0) setHovered(false);
        }}
        onDrop={(event) => {
          event.preventDefault();
          event.stopPropagation();
          endDrag();
          const id = droppedCreativeID(event.dataTransfer, disabled);
          if (disabled) return;
          if (id) select(id);
          else setMessage('不支持文件或外部链接，请拖入右侧素材库的小人。');
        }}
        className={
          'flex min-h-48 w-full flex-wrap items-center justify-center gap-4 rounded-xl border-2 border-dashed p-4 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary sm:flex-nowrap ' +
          (hovered && !disabled
            ? 'border-primary bg-primary/10'
            : 'border-border bg-muted/35') +
          (disabled ? ' opacity-60' : '')
        }
      >
        {selected && (
          <CreativeImage src={selected.path} alt={selected.name} size={160} />
        )}
        <span className="block min-w-0 space-y-2">
          <span className="block text-xs font-medium text-muted-foreground">
            待添加素材
          </span>
          <span className="block break-words text-lg font-medium">
            {hovered && !disabled
              ? '松开，放到左侧'
              : (selected?.name ?? '选择一份素材')}
          </span>
          <span className="block text-sm text-muted-foreground">
            从素材库选择或拖入图片。
          </span>
        </span>
      </button>
      <output className="block text-xs text-primary" aria-atomic="true">
        {message}
      </output>
    </div>
  );
}

export function CreativeAssetPicker({
  value,
  onChange,
  disabled = false,
  libraryRef,
}: PickerProps & { libraryRef: RefObject<HTMLDivElement | null> }) {
  const [dragging, setDragging] = useState<string | null>(null);
  function endDrag() {
    setDragging(null);
  }
  function select(id: string) {
    if (!disabled) onChange(id);
  }
  return (
    <div className="space-y-3">
      <p className="text-sm">本地素材（{demoCreatives.length}）</p>
      <div
        ref={libraryRef}
        className="grid max-h-[32rem] grid-cols-3 gap-2 overflow-y-auto p-1"
        aria-label="右侧本地素材库"
      >
        {demoCreatives.map((asset) => (
          <Button
            key={asset.id}
            type="button"
            variant={value === asset.id ? 'secondary' : 'outline'}
            disabled={disabled}
            draggable={!disabled}
            onDragStart={(event) => {
              if (disabled) {
                event.preventDefault();
                return;
              }
              event.dataTransfer.setData(CREATIVE_DRAG_TYPE, asset.id);
              event.dataTransfer.effectAllowed = 'copy';
              const preview = event.currentTarget.querySelector('img');
              if (preview)
                event.dataTransfer.setDragImage(
                  preview,
                  preview.width / 2,
                  preview.height / 2,
                );
              setDragging(asset.id);
            }}
            onDragEnd={endDrag}
            aria-pressed={value === asset.id}
            aria-label={'选择' + asset.name + '素材'}
            title={asset.name + ' · ' + asset.author}
            className={
              'relative h-auto cursor-grab flex-col gap-1 p-2 active:cursor-grabbing ' +
              (dragging === asset.id ? 'opacity-50' : '')
            }
            onClick={() => select(asset.id)}
          >
            <GripVertical className="absolute top-1 left-1 size-3 text-muted-foreground" />
            {value === asset.id && (
              <Check className="absolute top-1 right-1 size-3 text-primary" />
            )}
            <CreativeImage src={asset.path} alt={asset.name} size={64} />
            <span className="text-center text-xs whitespace-normal">
              {asset.name}
            </span>
          </Button>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        同人素材 · 对外使用前请确认授权
      </p>
    </div>
  );
}
