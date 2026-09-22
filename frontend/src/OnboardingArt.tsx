import type { ReactNode } from "react";
import type { BookLevel } from "./api.ts";
import styles from "./OnboardingArt.module.scss";

// Small line drawings for the borrow and return steps on the onboarding
// screen. Every drawing is a 72×72 square in the app's own palette.

// Alternating bar and gap widths; odd length so it ends on a bar.
const barcodePattern = [
  1, 1, 2, 1, 1, 2, 3, 1, 1, 1, 2, 2, 1, 1, 3, 1, 1, 2, 1,
];
const barcodeUnits = barcodePattern.reduce((sum, width) => sum + width, 0);
// Each bar's offset and width, in pattern units.
const barcodeBars = barcodePattern.flatMap((width, index) => {
  const offset = barcodePattern
    .slice(0, index)
    .reduce((sum, before) => sum + before, 0);
  return index % 2 === 0 ? [{ offset, width }] : [];
});

function Barcode({
  x,
  y,
  width,
  height,
}: {
  x: number;
  y: number;
  width: number;
  height: number;
}) {
  const unit = width / barcodeUnits;
  return (
    <g className={styles.ink}>
      {barcodeBars.map((bar) => (
        <rect
          key={bar.offset}
          x={x + bar.offset * unit}
          y={y}
          width={bar.width * unit}
          height={height}
        />
      ))}
    </g>
  );
}

// Corner brackets of a camera viewfinder around a box.
function Viewfinder({
  x,
  y,
  width,
  height,
}: {
  x: number;
  y: number;
  width: number;
  height: number;
}) {
  const arm = 5;
  const right = x + width;
  const bottom = y + height;
  return (
    <path
      className={styles.viewfinder}
      d={[
        `M${x} ${y + arm}V${y}H${x + arm}`,
        `M${right - arm} ${y}H${right}V${y + arm}`,
        `M${right} ${bottom - arm}V${bottom}H${right - arm}`,
        `M${x + arm} ${bottom}H${x}V${bottom - arm}`,
      ].join("")}
    />
  );
}

function Art({ children }: { children: ReactNode }) {
  return (
    <svg
      className={styles.art}
      viewBox="0 0 72 72"
      width="72"
      height="72"
      aria-hidden="true"
    >
      {children}
    </svg>
  );
}

const shelfTop = 60;

// One book standing on the shelf, seen from the spine: a cover colour, two
// rules and a sticker in its level colour.
function Spine({
  x,
  width,
  height,
  cover,
  level,
}: {
  x: number;
  width: number;
  height: number;
  cover: "sand" | "label" | "sandDeep";
  level: BookLevel;
}) {
  const top = shelfTop - height;
  return (
    <g>
      <rect
        className={styles[cover]}
        x={x}
        y={top}
        width={width}
        height={height}
      />
      <rect
        className={styles[level]}
        x={x + 0.75}
        y={top + 7}
        width={width - 1.5}
        height="4"
      />
      <path
        className={styles.rule}
        d={`M${x} ${top + 3.5}h${width}M${x} ${shelfTop - 3.5}h${width}`}
      />
    </g>
  );
}

// A shelf with one book lifted out of its place, or on its way back in.
export function ShelfArt({ direction }: { direction: "out" | "in" }) {
  const lift = direction === "out" ? -20 : -12;
  const tilt = direction === "out" ? -8 : 4;
  return (
    <Art>
      <Spine x={5} width={8} height={36} cover="sand" level="green" />
      <Spine x={13} width={10} height={30} cover="label" level="yellow" />
      <Spine x={23} width={7} height={40} cover="sandDeep" level="green" />
      <Spine x={41} width={9} height={38} cover="label" level="red" />
      <Spine x={50} width={7} height={31} cover="sand" level="yellow" />
      <Spine x={57} width={10} height={36} cover="sandDeep" level="green" />
      <g transform={`translate(0 ${lift}) rotate(${tilt} 35.5 ${shelfTop})`}>
        <Spine x={31} width={9} height={34} cover="label" level="yellow" />
      </g>
      <path
        className={styles.motion}
        d={
          direction === "out"
            ? "M33 46v5M36 48v7M39 46v5"
            : "M32 4v5M36 1v7M40 3v5"
        }
      />
      <rect className={styles.plank} x="1" y={shelfTop} width="70" height="4" />
      <path className={styles.bracket} d="M9 64v6h2l6-6M63 64v6h-2l-6-6" />
    </Art>
  );
}

// The back of a book with a phone reading its barcode.
export function PhoneScanArt() {
  return (
    <Art>
      <rect className={styles.sand} x="4" y="6" width="36" height="52" />
      <rect className={styles.label} x="14" y="38" width="20" height="14" />
      <Barcode x={16} y={40} width={16} height={8} />
      <rect
        className={styles.phone}
        x="36"
        y="16"
        width="30"
        height="52"
        rx="4"
      />
      <rect className={styles.ink} x="47" y="20" width="8" height="1.5" />
      <Barcode x={43} y={36} width={16} height={12} />
      <Viewfinder x={40} y={32} width={24} height={20} />
      <path className={styles.scanLine} d="M39 42H65" />
    </Art>
  );
}

// A calendar page counting the reading days.
export function DueDateArt() {
  return (
    <Art>
      <rect className={styles.label} x="10" y="12" width="46" height="50" />
      <rect className={styles.spine} x="10" y="12" width="46" height="12" />
      <rect className={styles.ink} x="20" y="7" width="3" height="10" />
      <rect className={styles.ink} x="43" y="7" width="3" height="10" />
      <text className={styles.number} x="33" y="51" textAnchor="middle">
        21
      </text>
      <circle className={styles.check} cx="58" cy="58" r="10" />
      <path className={styles.tick} d="M53.5 58.5l3 3 6-6.5" />
    </Art>
  );
}

// The cabinet door with the shelf QR code in a viewfinder.
export function ShelfCodeArt() {
  return (
    <Art>
      <rect className={styles.sand} x="8" y="4" width="52" height="64" />
      <rect className={styles.ink} x="52" y="30" width="3" height="14" />
      <rect className={styles.label} x="17" y="17" width="26" height="32" />
      <g className={styles.ink}>
        {[
          [20, 20],
          [34, 20],
          [20, 34],
        ].map(([x, y]) => (
          <path
            key={`${x}-${y}`}
            fillRule="evenodd"
            d={`M${x} ${y}h6v6h-6zM${x + 1} ${y + 1}v4h4v-4zM${x + 2} ${y + 2}h2v2h-2z`}
          />
        ))}
        {[
          [28, 20],
          [30, 22],
          [28, 24],
          [28, 28],
          [32, 28],
          [36, 28],
          [38, 30],
          [34, 32],
          [28, 32],
          [30, 36],
          [34, 36],
          [38, 34],
          [36, 38],
          [28, 38],
        ].map(([x, y]) => (
          <rect key={`${x}-${y}`} x={x} y={y} width="2" height="2" />
        ))}
      </g>
      <rect className={styles.muted} x="21" y="44" width="18" height="1.5" />
      <Viewfinder x={13} y={13} width={34} height={40} />
    </Art>
  );
}

// The back of a book with its barcode in a viewfinder.
export function BookBarcodeArt() {
  return (
    <Art>
      <rect className={styles.sand} x="12" y="4" width="48" height="64" />
      <rect className={styles.label} x="28" y="42" width="26" height="18" />
      <Barcode x={31} y={45} width={20} height={9} />
      <rect className={styles.muted} x="31" y="56" width="20" height="1.5" />
      <Viewfinder x={24} y={38} width={34} height={26} />
    </Art>
  );
}
