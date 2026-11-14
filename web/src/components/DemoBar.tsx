import styles from "./DemoBar.module.css";

// The byline: visitors who never open GitHub still see whose demo this is.
export function DemoBar() {
  return (
    <aside className={styles.bar} aria-label="Tentang demo ini">
      Demo portofolio oleh{" "}
      <a href="https://www.linkedin.com/in/jeevallucas-gautama/">
        Putu Jeevallucas Jnanamaitriya Surya Gautama
      </a>
      , November 2026. Venue fiktif; pembayaran tiruan.
    </aside>
  );
}
