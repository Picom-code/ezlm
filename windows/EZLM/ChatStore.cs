using System.Text.Json;

namespace EZLM;

/// Chats are kept on this computer (%LOCALAPPDATA%\EZLM\chats, one file per
/// conversation), so they're still there next time EZLM opens.
static class ChatStore
{
    public sealed class Saved
    {
        public string Id { get; set; } = "";
        public string Title { get; set; } = "";
        public string Model { get; set; } = "";
        public DateTime Updated { get; set; }
        public List<SavedMsg> Messages { get; set; } = new();
    }

    public sealed class SavedMsg
    {
        public bool User { get; set; }
        public string Model { get; set; } = "";
        public string Text { get; set; } = "";
        public string Thinking { get; set; } = "";
        public string Error { get; set; } = "";
        public bool Stopped { get; set; }
        public double Rate { get; set; }
        public int Tokens { get; set; }
        public double Secs { get; set; }
        public double ThinkSecs { get; set; }
        public int Left { get; set; }
        public bool Cut { get; set; }
        public List<SavedFile> Files { get; set; } = new();
    }

    public sealed class SavedFile
    {
        public string Name { get; set; } = "";
        public string Text { get; set; } = "";
    }

    /// Where chats and memory live; the self-test points this at a throwaway folder.
    internal static string Base => Environment.GetEnvironmentVariable("EZLM_CHAT_DIR") is { Length: > 0 } t ? t : Engine.DataDir;

    static string Dir => Path.Combine(Base, "chats");

    public static string NewId() => DateTime.Now.ToString("yyyyMMdd-HHmmss-fff");

    public static void Save(Saved s)
    {
        if (Engine.Demo) return; // the demo keeps nothing: never mix it into real chats
        try
        {
            Directory.CreateDirectory(Dir);
            var path = Path.Combine(Dir, s.Id + ".json");
            File.WriteAllText(path + ".tmp", JsonSerializer.Serialize(s, Engine.Json));
            File.Move(path + ".tmp", path, true); // whole or not at all
        }
        catch (Exception ex) { Shell.Log(ex); }
    }

    /// The most recent chats first (only the newest 40 are looked at).
    public static List<Saved> Recent()
    {
        if (Engine.Demo) return new();
        try
        {
            if (!Directory.Exists(Dir)) return new();
            return new DirectoryInfo(Dir).GetFiles("*.json").OrderByDescending(f => f.LastWriteTimeUtc).Take(40)
                .Select(f => Load(f.FullName)).OfType<Saved>().OrderByDescending(s => s.Updated).ToList();
        }
        catch (Exception ex) { Shell.Log(ex); return new(); }
    }

    static Saved? Load(string path)
    {
        try { return JsonSerializer.Deserialize<Saved>(File.ReadAllText(path), Engine.Json); }
        catch { return null; } // a damaged file just doesn't show
    }

    public static void Delete(string id)
    {
        try { File.Delete(Path.Combine(Dir, id + ".json")); } catch (Exception ex) { Shell.Log(ex); }
    }

    public static void DeleteAll()
    {
        try { if (Directory.Exists(Dir)) foreach (var f in Directory.GetFiles(Dir, "*.json")) File.Delete(f); } catch (Exception ex) { Shell.Log(ex); }
    }

    /// "2 min ago", "3 h ago", "yesterday", "Nov 1".
    public static string Ago(DateTime t)
    {
        var d = DateTime.Now - t;
        return d.TotalMinutes < 1 ? "just now" : d.TotalHours < 1 ? $"{(int)d.TotalMinutes} min ago" : d.TotalDays < 1 ? $"{(int)d.TotalHours} h ago"
            : d.TotalDays < 2 ? "yesterday" : t.ToString("MMM d");
    }
}

/// Chat's plain logic (commands, sizes, error wording): no UI, so the
/// app's self-test can check it.
static class ChatLogic
{
    internal static readonly (string Cmd, string Arg, string What)[] Commands =
    {
        ("/remember", " <fact>", "save something about you for future chats"),
        ("/forget", " <words>", "remove remembered things that mention these words"),
        ("/memory", "", "see and edit everything it remembers"),
        ("/new", "", "start a new chat"),
    };

    /// What Continue sends when an answer ran into the model's limit: these
    /// words, with the end of the answer attached. If the answer filled the
    /// model's memory, the whole of it can't go back, but the model still
    /// needs to see where it stopped.
    internal const string ContinueText = "Continue exactly from where you stopped, without repeating anything.";
    internal static (string Name, string Text) AnswerEnd(string answer) =>
        ("end of the last answer", answer.Length > 1200 ? "…" + answer[^1200..] : answer);

    internal static string Short(long n) => n >= 1000 ? $"{n / 1000.0:0.#}k" : n.ToString();

    /// About how many tokens text is, on the safe side: English words run
    /// about 4 letters a token, but digits, punctuation and line breaks (most
    /// of a table of numbers or code) about one each; other alphabets 2 letters,
    /// and Chinese, Japanese or Korean about one a character.
    internal static int Tokens(string text)
    {
        int letters = 0, marks = 0, wide = 0, other = 0;
        foreach (var ch in text)
            if (char.IsAsciiLetter(ch) || ch == ' ') letters++; else if (ch < 0x80) marks++; else if (ch >= 0x2E80) wide++; else other++;
        return (int)(letters / 4.0 + marks + wide + other / 2.0);
    }

    /// What a model that reads `ctx` tokens at once has for the conversation,
    /// after keeping room for its answer.
    internal static int Room(int ctx) => ctx - Math.Clamp(ctx / 4, 512, 4096);

    /// Keeps a conversation inside what the model can read at once: the newest
    /// turns stay and the oldest go, so a long chat keeps going instead of
    /// stopping. What's kept starts with a question, so turns still alternate,
    /// and an answer at the end keeps its question (the memory check). The
    /// starting point moves in steps of about a third of the room, not every
    /// turn, so the model reuses what it already read for several turns
    /// instead of reading the whole conversation again for each answer.
    /// `notes` is what goes along anyway (the memory notes); a `share` below 1
    /// squeezes harder after the model said it was still too long. Returns
    /// how many messages were left out.
    internal static int Fit(List<(string Role, string Content)> turns, int ctx, int notes = 0, double share = 1)
    {
        if (ctx <= 0 || turns.Count <= 1) return 0;
        double room = (Room(ctx) - notes) * share;
        var cost = turns.Select(t => Tokens(t.Content) + 8.0).ToList(); // each turn's role markers cost a few tokens
        if (cost.Sum() <= room) return 0;
        int must = turns.Count - 1; // the newest message always goes, with its question if it's an answer
        if (must > 0 && turns[must].Role != "user") must--;
        int min = must;
        double used = 0;
        for (int i = turns.Count - 1; i >= 0; i--)
        {
            used += cost[i];
            if (used > room && i < must) break;
            min = i;
        }
        // The first step boundary at or after `min`: steps are counted from the
        // start of the chat, so the boundary stays put while the chat grows.
        double step = Math.Max(1, room / 3), before = 0, band = -1;
        int start = min;
        for (int i = 0; i <= must; i++)
        {
            double b = Math.Floor(before / step);
            if (i >= min && b > band) { start = i; break; }
            band = b;
            before += cost[i];
        }
        while (start < must && turns[start].Role != "user") start++;
        turns.RemoveRange(0, start);
        return start;
    }

    /// The messages for the model: the notes about the user first (fixed for
    /// the chat), then the turns before `upto` that fit (Fit). A question whose
    /// answer failed is left out, since many models need user and assistant
    /// turns to alternate. `inline` folds the notes into the first question,
    /// for chat formats with no place for them. `chat` is each message as the
    /// model reads it (a question with its files). Says how many were left out.
    internal static List<(string Role, string Content)> Messages(IReadOnlyList<(bool User, string Content)> chat, int upto, string? notes, int ctx, bool inline, double share, out int left)
    {
        var h = new List<(string Role, string Content)>();
        for (int i = 0; i < upto; i++)
        {
            var (user, content) = chat[i];
            if (user && (i == upto - 1 || (i + 1 < chat.Count && !chat[i + 1].User && chat[i + 1].Content.Length > 0))) h.Add(("user", content));
            else if (!user && content.Length > 0) h.Add(("assistant", content));
        }
        left = Fit(h, ctx, notes == null ? 0 : Tokens(notes), share);
        if (notes != null)
        {
            if (!inline) h.Insert(0, ("system", notes));
            else if (h.FindIndex(x => x.Role == "user") is var u and >= 0) h[u] = ("user", notes + "\n" + h[u].Content);
        }
        return h;
    }

    internal enum Refusal { Inline, Squeeze, Fail }

    /// What to do when the model turns a request down: fold the notes into the
    /// question (its chat format has no place for them), leave out more of the
    /// oldest turns (still too long by its own count), or show the error.
    internal static Refusal AfterRefusal(string error, bool inline, bool hasNotes, int squeezes, int questions) =>
        !inline && hasNotes && error.Contains("system", StringComparison.OrdinalIgnoreCase) ? Refusal.Inline
        : TooLong(error) && squeezes < 3 && questions > 1 ? Refusal.Squeeze
        : Refusal.Fail;

    /// The model's own word that what it was sent is more than it can read at once.
    internal static bool TooLong(string error) => error.Contains("context", StringComparison.OrdinalIgnoreCase);

    /// After the model said what was sent is too long: how much of the room to
    /// fill on the next try. llama-server says how many tokens it really was,
    /// so the guesses are scaled to match and the next try lands a little
    /// under the room; without that count, it halves.
    internal static double Squeeze(string error, List<(string Role, string Content)> sent, int ctx, int notes, double share)
    {
        double guess = sent.Sum(t => Tokens(t.Content) + 8), real = 0;
        try
        {
            using var doc = JsonDocument.Parse(error);
            if (doc.RootElement.TryGetProperty("error", out var e) && e.ValueKind == JsonValueKind.Object && e.TryGetProperty("n_prompt_tokens", out var n) && n.TryGetInt32(out var count)) real = count;
        }
        catch (JsonException) { }
        if (real <= guess || guess <= 0) return share / 2;
        double room = Room(ctx), aim = (0.9 * room * guess / real - notes) / Math.Max(1, room - notes);
        return Math.Min(Math.Max(aim, 0.01), share * 0.8); // always tighter than last time
    }

    internal static List<(string Cmd, string Arg, string What)> Matches(string draft)
    {
        if (!draft.StartsWith('/') || draft.Contains(' ') || draft.Contains('\n')) return new();
        return Commands.Where(c => c.Cmd.StartsWith(draft, StringComparison.OrdinalIgnoreCase)).ToList();
    }

    /// llama-server's error, in words a person can act on.
    internal static string ChatError(int code, string body)
    {
        var msg = body;
        try
        {
            using var doc = JsonDocument.Parse(body);
            if (doc.RootElement.TryGetProperty("error", out var e))
                msg = e.ValueKind == JsonValueKind.Object && e.TryGetProperty("message", out var m) ? m.GetString() ?? body : e.ToString();
        }
        catch (JsonException) { }
        var low = msg.ToLowerInvariant();
        if (low.Contains("context")) return "That's more than the model can read at once, even with older messages left out. Try a shorter message or file.";
        if (code == 503 || low.Contains("loading")) return "Your model is still loading. Try again in a moment.";
        if (code == 401) return "The server didn't accept EZLM's key. Restart EZLM; if it keeps happening, report a problem.";
        return $"Your model answered with an error ({code}): {msg}";
    }
}
