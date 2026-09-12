import type { AnchorHTMLAttributes, MouseEvent } from "react";
import { navigate } from "../router.ts";

type Props = AnchorHTMLAttributes<HTMLAnchorElement> & { href: string };

// Link moves between app screens without a page load. Paths under /api/ are
// server endpoints (the OAuth redirects), so they load like ordinary links,
// as do other sites and modified clicks (new tab, new window).
export function Link({ href, onClick, ...rest }: Props) {
  function handleClick(event: MouseEvent<HTMLAnchorElement>) {
    onClick?.(event);
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey ||
      rest.target ||
      !href.startsWith("/") ||
      href.startsWith("/api/")
    ) {
      return;
    }
    event.preventDefault();
    navigate(href);
  }

  return <a {...rest} href={href} onClick={handleClick} />;
}
