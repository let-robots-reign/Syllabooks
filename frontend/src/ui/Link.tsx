import type { AnchorHTMLAttributes } from "react";
import { Link as RouterLink } from "react-router-dom";

type Props = AnchorHTMLAttributes<HTMLAnchorElement> & { href: string };

// App routes use React Router. API endpoints and external URLs remain normal
// anchors because they must perform a full document navigation.
export function Link({ href, ...rest }: Props) {
  if (!href.startsWith("/") || href.startsWith("/api/")) {
    return <a {...rest} href={href} />;
  }
  return <RouterLink {...rest} to={href} />;
}
