import { NextResponse } from "next/server";
import { getChartData } from "@/lib/usageDb";
import { getApiKeyById } from "@/lib/db/repos/apiKeysRepo";

const VALID_PERIODS = new Set(["today", "24h", "7d", "30d", "60d"]);

export const dynamic = "force-dynamic";

export async function GET(request, { params }) {
  try {
    const { keyId } = await params;
    const { searchParams } = new URL(request.url);
    const period = searchParams.get("period") || "7d";

    if (!VALID_PERIODS.has(period)) {
      return NextResponse.json({ error: "Invalid period" }, { status: 400 });
    }

    const key = await getApiKeyById(keyId);
    if (!key) {
      return NextResponse.json({ error: "API key not found" }, { status: 404 });
    }

    const chartData = await getChartData(period, { apiKey: key.key });
    return NextResponse.json({ keyId: key.id, period, chartData });
  } catch (error) {
    console.error("[API] Failed to get per-key chart:", error);
    return NextResponse.json({ error: "Failed to fetch per-key chart" }, { status: 500 });
  }
}
