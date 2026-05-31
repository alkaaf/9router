import { NextResponse } from "next/server";
import { getUsageHistory } from "@/lib/usageDb";
import { getApiKeyById } from "@/lib/db/repos/apiKeysRepo";

export const dynamic = "force-dynamic";

export async function GET(request, { params }) {
  try {
    const { keyId } = await params;
    const { searchParams } = new URL(request.url);
    const limit = Math.min(parseInt(searchParams.get("limit") || "50", 10), 200);
    const offset = parseInt(searchParams.get("offset") || "0", 10);

    const key = await getApiKeyById(keyId);
    if (!key) {
      return NextResponse.json({ error: "API key not found" }, { status: 404 });
    }

    const history = await getUsageHistory({ apiKey: key.key, limit, offset });
    return NextResponse.json({ keyId: key.id, history, limit, offset });
  } catch (error) {
    console.error("[API] Failed to get per-key history:", error);
    return NextResponse.json({ error: "Failed to fetch per-key history" }, { status: 500 });
  }
}
