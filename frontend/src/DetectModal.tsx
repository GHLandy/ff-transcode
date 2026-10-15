import { useEffect } from 'react';
import type { DetectSourceKind, ToolDetection } from './types';

/** 每个查找来源对应的结果图标样式。 */
const SOURCE_CLASS: Record<DetectSourceKind, string> = {
  user: 'src-user',
  bin: 'src-bin',
  path: 'src-path',
};

interface Props {
  detection: ToolDetection | null;
  onClose: () => void;
  onChoose: () => void;
  onClear: () => void;
  onOpenBinDir: () => void;
  onDownloadFFmpeg: () => void;
}

export default function DetectModal({
  detection,
  onClose,
  onChoose,
  onClear,
  onOpenBinDir,
  onDownloadFFmpeg,
}: Props) {
  // Esc 关闭 + 打开时锁定滚动。
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const ok = detection?.available ?? false;
  const hasUserSetting = detection?.sources.some((s) => s.kind === 'user' && s.found) ?? false;
  const binDir = detection?.binDir || '';

  return (
    <div className="modal-mask" onClick={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-label="FFmpeg 检测详情"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-head">
          <h3>FFmpeg 检测详情</h3>
          <button className="modal-close" onClick={onClose} type="button" aria-label="关闭">
            ×
          </button>
        </div>

        <div className="modal-body">
          <div className={`detect-state ${ok ? 'ok' : 'bad'}`}>
            <span className="dot" />
            <b>
              {ok
                ? '已找到 ffmpeg / ffprobe，可以开始转码'
                : detection?.message || '未找到 ffmpeg / ffprobe'}
            </b>
          </div>

          <p className="detect-desc">
            需要在同一个目录中<strong>同时找到</strong> <code>ffmpeg</code> 与 <code>ffprobe</code>
            （两者随 FFmpeg 官方发行包一起提供，版本必须配套）。
            <br />
            查找顺序：<b>用户设置</b> → <b>程序 bin 目录</b> → <b>PATH</b>。
          </p>

          {!ok && (
            <>
              <div className="detect-sub">请任选一种方式完成设置：</div>
              <ol className="detect-steps">
                <li>
                  <b>放入程序 bin 目录</b>：下载 FFmpeg 后，将 <code>ffmpeg.exe</code> 与{' '}
                  <code>ffprobe.exe</code> 复制到 <code>{binDir || '程序目录\\bin'}</code>
                  <button className="btn tiny" onClick={onOpenBinDir} type="button">
                    打开该目录
                  </button>
                </li>
                <li>
                  <b>加入系统 PATH</b>：将 FFmpeg 的 bin 目录添加到系统 PATH
                  环境变量，重启应用后重新检测。
                </li>
                <li>
                  <b>手动选择</b>：直接指定 ffmpeg 可执行文件的位置（同目录须存在 ffprobe）。
                </li>
              </ol>
            </>
          )}

          <div className="detect-actions">
            <button className="btn primary" onClick={onChoose} type="button">
              手动选择 ffmpeg 位置
            </button>
            {hasUserSetting && (
              <button className="btn danger" onClick={onClear} type="button">
                清除设置
              </button>
            )}
            {!ok && (
              <button className="btn" onClick={onDownloadFFmpeg} type="button">
                下载 FFmpeg
              </button>
            )}
          </div>

          <div className="detect-sub">查找过程</div>
          <ul className="detect-sources">
            {(detection?.sources ?? []).map((s, i) => (
              <li key={`${s.kind}-${i}`} className={SOURCE_CLASS[s.kind] ?? ''}>
                <span className={`dot ${s.found ? 'ok' : 'bad'}`} />
                <span className="src-label">{s.label}</span>
                <span className="src-detail" title={s.detail || '-'}>
                  {s.detail || '-'}
                </span>
                <span className={`src-note ${s.found ? 'ok' : 'bad'}`}>{s.note}</span>
              </li>
            ))}
            {(!detection || detection.sources.length === 0) && (
              <li>
                <span className="dot bad" />
                <span className="src-label">尚未检测</span>
                <span className="src-detail">-</span>
                <span className="src-note bad">点击「重新检测」</span>
              </li>
            )}
          </ul>
        </div>
      </div>
    </div>
  );
}
