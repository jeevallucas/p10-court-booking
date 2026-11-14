import styles from "./SlowServerNote.module.css";

// The demo API sleeps after 15 idle minutes on its free host and needs about a minute to wake
// (docs/decisions.md). The note stays hidden unless a load has already taken five seconds.
export function SlowServerNote() {
  return (
    <p className={styles.note} role="status">
      Server demo sedang bangun setelah tidak dipakai (paket hosting gratis). Biasanya siap dalam
      satu menit; halaman ini lanjut sendiri.
    </p>
  );
}
