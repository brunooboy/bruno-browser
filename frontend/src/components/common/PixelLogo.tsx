import browserIcon from '../../assets/bruno-browser-icon.png'

const glyphs: Record<string, string[]> = {
  B: ['11110', '10001', '10001', '11110', '10001', '10001', '11110'],
  R: ['11110', '10001', '10001', '11110', '10100', '10010', '10001'],
  U: ['10001', '10001', '10001', '10001', '10001', '10001', '01110'],
  N: ['10001', '11001', '11001', '10101', '10011', '10011', '10001'],
  O: ['01110', '10001', '10001', '10001', '10001', '10001', '01110'],
  W: ['10001', '10001', '10001', '10101', '10101', '10101', '01010'],
  S: ['01111', '10000', '10000', '01110', '00001', '00001', '11110'],
  E: ['11111', '10000', '10000', '11110', '10000', '10000', '11111'],
}

function PixelWord({ word, y, color }: { word: string; y: number; color: string }) {
  const pixel = 1.8
  const letterWidth = pixel * 5
  const gap = 2.4

  return word.split('').flatMap((letter, letterIndex) =>
    (glyphs[letter] ?? []).flatMap((row, rowIndex) =>
      row.split('').map((cell, columnIndex) =>
        cell === '1' ? (
          <rect
            fill={color}
            height={pixel}
            key={`${letter}-${letterIndex}-${rowIndex}-${columnIndex}`}
            width={pixel}
            x={letterIndex * (letterWidth + gap) + columnIndex * pixel}
            y={y + rowIndex * pixel}
          />
        ) : null,
      ),
    ),
  )
}

export function PixelLogo({ compact = false }: { compact?: boolean }) {
  return (
    <div className={`pixel-logo ${compact ? 'pixel-logo--compact' : ''}`}>
      <img alt="" aria-hidden="true" className="pixel-logo__mark" src={browserIcon} />
      {!compact && <div className="pixel-logo__wordmark">
        <svg aria-label="Bruno Browser" role="img" viewBox="0 0 77 31">
          <PixelWord color="#42ff91" word="BRUNO" y={1} />
          <PixelWord color="#dce7e2" word="BROWSER" y={17} />
        </svg>
        <span className="pixel-logo__caption">LOCAL OPS // 1.4</span>
      </div>}
    </div>
  )
}
