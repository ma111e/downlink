#!/usr/bin/env python3
"""
Migrate feed IDs from MD5(full_url) to MD5(registered_domain).

Usage:
    pip install tldextract
    python migrate_feed_ids.py --db /path/to/downlink.db [--dry-run]
"""

import argparse
import hashlib
import sqlite3
import sys

try:
    import tldextract
except ImportError:
    print("error: tldextract is required. Run: pip install tldextract", file=sys.stderr)
    sys.exit(1)


def registered_domain(url: str) -> str:
    extracted = tldextract.extract(url)
    if not extracted.domain or not extracted.suffix:
        raise ValueError(f"cannot extract registered domain from URL: {url!r}")
    return f"{extracted.domain}.{extracted.suffix}"


def new_feed_id(url: str) -> str:
    domain = registered_domain(url)
    return hashlib.md5(domain.encode()).hexdigest()


def migrate(db_path: str, dry_run: bool) -> None:
    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    cur = conn.cursor()

    feeds = cur.execute("SELECT id, url, title FROM feeds").fetchall()
    if not feeds:
        print("no feeds found")
        return

    # Build the mapping: old_id -> (new_id, url, title)
    mapping: list[tuple[str, str, str, str]] = []
    errors: list[str] = []

    for feed in feeds:
        old_id = feed["id"]
        url = feed["url"]
        title = feed["title"] or url
        try:
            nid = new_feed_id(url)
        except ValueError as e:
            errors.append(str(e))
            continue
        mapping.append((old_id, nid, url, title))

    if errors:
        for e in errors:
            print(f"error: {e}", file=sys.stderr)
        sys.exit(1)

    skipped_same = [(old, nid, url, title) for old, nid, url, title in mapping if old == nid]
    to_update = [(old, nid, url, title) for old, nid, url, title in mapping if old != nid]

    # Check for collisions: two feeds with different old IDs mapping to the same new ID
    seen_new: dict[str, tuple[str, str]] = {}
    collisions: list[str] = []
    for old, nid, url, title in to_update:
        if nid in seen_new:
            other_old, other_title = seen_new[nid]
            collisions.append(
                f"collision: {title!r} ({old}) and {other_title!r} ({other_old}) both map to {nid}"
            )
        else:
            seen_new[nid] = (old, title)

    # Also check if a new ID already exists in the DB under a different old ID that we're not updating
    existing_ids = {feed["id"] for feed in feeds}
    for old, nid, url, title in to_update:
        if nid in existing_ids and nid != old:
            collisions.append(
                f"collision: new id {nid} for {title!r} ({old}) already exists in feeds table"
            )

    if collisions:
        for c in collisions:
            print(f"error: {c}", file=sys.stderr)
        sys.exit(1)

    print(f"feeds to update : {len(to_update)}")
    print(f"feeds unchanged : {len(skipped_same)}")
    if dry_run:
        for old, nid, url, title in to_update:
            dom = registered_domain(url)
            print(f"  {title!r}: {old} -> {nid}  (domain: {dom})")
        print("dry-run: no changes written")
        return

    # Apply updates in a single transaction with FK enforcement off
    try:
        cur.execute("PRAGMA foreign_keys = OFF")
        conn.execute("BEGIN")

        for old, nid, url, title in to_update:
            print(f"  updating {title!r}: {old} -> {nid}")

            # Articles: rewrite id prefix and feed_id
            cur.execute(
                "UPDATE articles SET id = ? || SUBSTR(id, LENGTH(?) + 1), feed_id = ? WHERE feed_id = ?",
                (nid, old, nid, old),
            )
            # article_tags
            cur.execute(
                "UPDATE article_tags SET article_id = ? || SUBSTR(article_id, LENGTH(?) + 1) WHERE article_id LIKE ?",
                (nid, old, old + ":%"),
            )
            # digest_articles
            cur.execute(
                "UPDATE digest_articles SET article_id = ? || SUBSTR(article_id, LENGTH(?) + 1) WHERE article_id LIKE ?",
                (nid, old, old + ":%"),
            )
            # article_analyses
            cur.execute(
                "UPDATE article_analyses SET article_id = ? || SUBSTR(article_id, LENGTH(?) + 1) WHERE article_id LIKE ?",
                (nid, old, old + ":%"),
            )
            # digest_analyses
            cur.execute(
                "UPDATE digest_analyses SET article_id = ? || SUBSTR(article_id, LENGTH(?) + 1) WHERE article_id LIKE ?",
                (nid, old, old + ":%"),
            )
            # related_articles (both columns)
            cur.execute(
                "UPDATE related_articles SET article_id = ? || SUBSTR(article_id, LENGTH(?) + 1) WHERE article_id LIKE ?",
                (nid, old, old + ":%"),
            )
            cur.execute(
                "UPDATE related_articles SET related_article_id = ? || SUBSTR(related_article_id, LENGTH(?) + 1) WHERE related_article_id LIKE ?",
                (nid, old, old + ":%"),
            )
            # feeds (must come last — PK update)
            cur.execute("UPDATE feeds SET id = ? WHERE id = ?", (nid, old))

        conn.commit()
        cur.execute("PRAGMA foreign_keys = ON")
        print(f"done: {len(to_update)} feed(s) updated")
    except Exception as e:
        conn.rollback()
        print(f"error: {e}", file=sys.stderr)
        sys.exit(1)
    finally:
        conn.close()


def main() -> None:
    parser = argparse.ArgumentParser(description="Migrate downlink feed IDs to domain-based hashes")
    parser.add_argument("--db", required=True, help="path to downlink.db")
    parser.add_argument("--dry-run", action="store_true", help="print changes without writing")
    args = parser.parse_args()

    migrate(args.db, args.dry_run)


if __name__ == "__main__":
    main()
