// 全卡覆盖率的"未实现"判定。
//
// 导入器写入的骨架是"effect 块里只有一条 unplayable;"。真正无法使用的卡（例如未来核心、
// 过往核心）还带有融合块，所以不能只看文件里有没有 unplayable。
export function isPlaceholder(text) {
  const open = text.indexOf('effect {');
  if (open < 0) return false;
  let depth = 0;
  let end = -1;
  for (let i = text.indexOf('{', open); i < text.length; i++) {
    if (text[i] === '{') depth++;
    else if (text[i] === '}') {
      depth--;
      if (depth === 0) {
        end = i;
        break;
      }
    }
  }
  if (end < 0) return false;
  const body = text
    .slice(text.indexOf('{', open) + 1, end)
    .replace(/<<[^\n]*/g, '')
    .replace(/\s+/g, '');
  return body === 'unplayable;';
}
