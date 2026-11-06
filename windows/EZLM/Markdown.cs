using System.Text;
using System.Text.RegularExpressions;
using Microsoft.UI.Text;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Documents;
using Windows.UI;
using static EZLM.Ui;

namespace EZLM;

/// A model's markdown answer drawn in EZLM's style: headings, lists, quotes,
/// code panels with light highlighting and a copy button, and tables drawn
/// with box lines. While an answer streams in, finished blocks are kept and
/// only the one still growing is rebuilt.
public sealed class MdView
{
    public readonly StackPanel Root = Col(10);
    readonly List<string> keys = new();
    static readonly Color Body = C(0xd6d6d6), Strong = C(0xffffff);

    /// Shows text; a block cursor follows the last word while streaming.
    public void Update(string text, bool streaming)
    {
        var blocks = MdParse.Parse(text);
        for (int i = 0; i < blocks.Count; i++)
        {
            bool cursor = streaming && i == blocks.Count - 1;
            var b = blocks[i];
            var key = $"{b.Kind}|{b.Level}|{b.Marker}|{b.Lang}|{b.Open}|{cursor}|{b.Text}";
            if (i < keys.Count && keys[i] == key) continue;
            var el = Build(b, cursor);
            if (i < keys.Count) { Root.Children[i] = el; keys[i] = key; }
            else { Root.Children.Add(el); keys.Add(key); }
        }
        while (keys.Count > blocks.Count) { Root.Children.RemoveAt(keys.Count - 1); keys.RemoveAt(keys.Count - 1); }
        if (blocks.Count == 0 && streaming) { Root.Children.Add(Para("", true)); keys.Add("cursor"); }
    }

    // MARK: building

    static FrameworkElement Build(Block b, bool cursor) => b.Kind switch
    {
        BlockKind.Heading => Para(b.Text, cursor, b.Level <= 2 ? 16 : 14, true).Also(t => t.Margin = new Thickness(0, b.Level <= 2 ? 8 : 4, 0, 0)),
        BlockKind.Item => Item(b, cursor),
        BlockKind.Quote => Quote(b.Text, cursor),
        BlockKind.Code => CodePanel(b.Text, b.Lang, cursor && b.Open),
        BlockKind.Table => Table(b.Text),
        BlockKind.Rule => Hairline().Also(h => h.Margin = new Thickness(0, 6, 0, 6)),
        _ => Para(b.Text, cursor),
    };

    static TextBlock Para(string text, bool cursor, double size = 13, bool bold = false)
    {
        var t = new TextBlock { FontFamily = Mono, FontSize = size, TextWrapping = TextWrapping.Wrap, IsTextSelectionEnabled = true, Foreground = B(bold ? Strong : Body), LineHeight = size * 1.55 };
        if (bold) t.FontWeight = FontWeights.Bold;
        Inline(t.Inlines, text);
        if (cursor) t.Inlines.Add(Cursor());
        return t;
    }

    /// The block cursor that follows the words while they're being written.
    public static Run Cursor() => new() { Text = " ▌", Foreground = B(Signal) };

    static FrameworkElement Item(Block b, bool cursor)
    {
        var g = new Grid { Margin = new Thickness(b.Level * 18, 0, 0, 0), ColumnSpacing = 10 };
        g.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        g.Children.Add(Txt(b.Marker, 13, b.Marker == "•" ? Signal : Dim, false, false));
        var t = Para(b.Text, cursor);
        Grid.SetColumn(t, 1); g.Children.Add(t);
        return g;
    }

    static FrameworkElement Quote(string text, bool cursor)
    {
        var g = new Grid { ColumnSpacing = 12 };
        g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(2) });
        g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        g.Children.Add(new Border { Background = B(Faint) });
        var t = Para(text, cursor);
        t.Foreground = B(Dim);
        Grid.SetColumn(t, 1); g.Children.Add(t);
        return g;
    }

    /// A code panel: language and Copy on top, highlighted code below that
    /// scrolls sideways instead of wrapping.
    static FrameworkElement CodePanel(string code, string lang, bool cursor)
    {
        var head = new Grid { Padding = new Thickness(12, 6, 6, 6), Background = B(C(0x141414)) };
        head.Children.Add(Txt(lang.Length > 0 ? lang : "code", 11, Dim, false, false).Also(t => t.VerticalAlignment = VerticalAlignment.Center));
        if (!cursor)
        {
            Button? cb = null;
            cb = Btn("Copy", Kind.Small, async () => { Copy(code); ((TextBlock)cb!.Content).Text = "Copied"; await Task.Delay(1400); ((TextBlock)cb.Content).Text = "Copy"; });
            cb.HorizontalAlignment = HorizontalAlignment.Right;
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(cb, "Copy code");
            head.Children.Add(cb);
        }
        var t = new TextBlock { FontFamily = Mono, FontSize = 12.5, IsTextSelectionEnabled = true, TextWrapping = TextWrapping.NoWrap, Foreground = B(Body), LineHeight = 19 };
        // While it's still being written, plain text: highlighting a growing block every frame is what made long answers stutter.
        if (cursor) t.Inlines.Add(new Run { Text = code }); else Highlight(t.Inlines, code, lang);
        if (cursor) t.Inlines.Add(Cursor());
        var scroll = new ScrollViewer { Content = t, HorizontalScrollBarVisibility = ScrollBarVisibility.Auto, VerticalScrollBarVisibility = ScrollBarVisibility.Disabled, HorizontalScrollMode = ScrollMode.Auto, VerticalScrollMode = ScrollMode.Disabled, Padding = new Thickness(14, 10, 14, 12) };
        return new Border { BorderBrush = B(Rule), BorderThickness = new Thickness(1), Background = B(Ui.Panel), Child = Col(0, head, scroll) };
    }

    /// Tables as box drawings: columns padded to line up, header set apart.
    static FrameworkElement Table(string text)
    {
        var rows = text.Split('\n')
            .Select(r => r.Trim().Trim('|').Split('|').Select(c => c.Trim()).ToList())
            .Where(r => !r.All(c => Regex.IsMatch(c, @"^:?-{2,}:?$")))
            .ToList();
        if (rows.Count == 0) return new Border();
        int n = rows.Max(r => r.Count);
        var w = new int[n];
        foreach (var r in rows) for (int c = 0; c < r.Count; c++) w[c] = Math.Min(48, Math.Max(w[c], r[c].Length));
        string Line(char l, char m, char r) => l + string.Join(m, w.Select(x => new string('─', x + 2))) + r;
        string Row(List<string> r) => "│" + string.Join("│", Enumerable.Range(0, n).Select(c =>
        {
            var cell = c < r.Count ? r[c] : "";
            if (cell.Length > w[c]) cell = cell[..(w[c] - 1)] + "…";
            return " " + cell.PadRight(w[c]) + " ";
        })) + "│";
        // Line height = font size, so the box lines join up.
        var t = new TextBlock { FontFamily = Mono, FontSize = 13, IsTextSelectionEnabled = true, TextWrapping = TextWrapping.NoWrap, LineHeight = 17, LineStackingStrategy = LineStackingStrategy.BlockLineHeight };
        void Add(string s, Color c, bool bold = false) { t.Inlines.Add(new Run { Text = s, Foreground = B(c), FontWeight = bold ? FontWeights.Bold : FontWeights.Normal }); t.Inlines.Add(new LineBreak()); }
        Add(Line('┌', '┬', '┐'), Dim);
        Add(Row(rows[0]), Strong, true);
        if (rows.Count > 1) Add(Line('├', '┼', '┤'), Dim);
        foreach (var r in rows.Skip(1)) Add(Row(r), Body);
        t.Inlines.Add(new Run { Text = Line('└', '┴', '┘'), Foreground = B(Dim) });
        return new ScrollViewer { Content = t, HorizontalScrollBarVisibility = ScrollBarVisibility.Auto, VerticalScrollBarVisibility = ScrollBarVisibility.Disabled, HorizontalScrollMode = ScrollMode.Auto, VerticalScrollMode = ScrollMode.Disabled };
    }

    // MARK: inline text

    static readonly Regex InlineRx = new(@"(`[^`\n]+`)|(\*\*[^*\n]+\*\*)|(__[^_\n]+__)|((?<![\w*])\*[^*\s][^*\n]*\*(?!\w))|((?<!\w)_[^_\s][^_\n]*_(?!\w))|(\[[^\]\n]+\]\((https?://[^)\s]+)\))");

    /// **bold**, *italic*, `code` and [links](https://…); nothing else is special.
    static void Inline(InlineCollection to, string text)
    {
        int at = 0;
        foreach (Match m in InlineRx.Matches(text))
        {
            if (m.Index > at) to.Add(new Run { Text = text[at..m.Index] });
            var v = m.Value;
            if (m.Groups[1].Success) to.Add(new Run { Text = v[1..^1], Foreground = B(Signal) });
            else if (m.Groups[2].Success || m.Groups[3].Success) to.Add(new Run { Text = v[2..^2], FontWeight = FontWeights.Bold, Foreground = B(Strong) });
            else if (m.Groups[4].Success || m.Groups[5].Success) to.Add(new Run { Text = v[1..^1], FontStyle = Windows.UI.Text.FontStyle.Italic });
            else if (m.Groups[6].Success)
            {
                var label = v[1..v.IndexOf("](", StringComparison.Ordinal)];
                if (Uri.TryCreate(m.Groups[7].Value, UriKind.Absolute, out var uri) && uri.Scheme is "http" or "https")
                {
                    var link = new Hyperlink { NavigateUri = uri, Foreground = B(Signal) };
                    link.Inlines.Add(new Run { Text = label });
                    to.Add(link);
                }
                else to.Add(new Run { Text = label });
            }
            at = m.Index + m.Length;
        }
        if (at < text.Length) to.Add(new Run { Text = text[at..] });
    }

    // MARK: code highlighting

    static readonly HashSet<string> Keywords = new(("if else elif for while do return def class function func fn const let var mut import from export " +
        "public private protected static void int long float double string bool char byte true false null nil None True False async await new try catch " +
        "finally raise throw throws with as in is not and or package type struct interface enum impl use pub match case switch break continue default " +
        "yield lambda self this super extends implements namespace using select where echo then fi done local").Split(' '));

    static readonly Regex CodeRx = new(@"(?<c>//.*$|#(?![\w{]).*$|--\s.*$)|(?<s>""(?:[^""\\]|\\.)*""?|'(?:[^'\\\n]|\\.)*'?|`[^`]*`?)|(?<n>\b\d+(?:\.\d+)?\b)|(?<w>[A-Za-z_][A-Za-z0-9_]*)");

    /// A light touch: keywords, strings, numbers and comments, whatever the language.
    static void Highlight(InlineCollection to, string code, string lang)
    {
        bool hashComments = lang is "" or "python" or "py" or "bash" or "sh" or "shell" or "zsh" or "ruby" or "rb" or "yaml" or "yml" or "toml" or "powershell" or "ps1" or "r" or "perl";
        var lines = code.Split('\n');
        for (int li = 0; li < lines.Length; li++)
        {
            if (li > 0) to.Add(new LineBreak());
            var line = lines[li];
            int at = 0;
            foreach (Match m in CodeRx.Matches(line))
            {
                Color? c = null;
                if (m.Groups["c"].Success && (m.Value[0] != '#' || hashComments)) c = Dim;
                else if (m.Groups["s"].Success) c = Ok;
                else if (m.Groups["n"].Success) c = Gold;
                else if (m.Groups["w"].Success && Keywords.Contains(m.Value)) c = Signal;
                if (c == null) continue;
                if (m.Index > at) to.Add(new Run { Text = line[at..m.Index] });
                to.Add(new Run { Text = m.Value, Foreground = B(c.Value) });
                at = m.Index + m.Length;
            }
            if (at < line.Length) to.Add(new Run { Text = line[at..] });
        }
    }
}

/// The block structure of a markdown answer: plain logic, no UI, so the
/// app's self-test can check it.
enum BlockKind { Para, Heading, Item, Quote, Code, Table, Rule }

sealed record Block(BlockKind Kind, string Text, int Level = 0, string Marker = "", string Lang = "", bool Open = false);

static class MdParse
{
    static readonly Regex HeadingRx = new(@"^(#{1,6})\s+(.*)$"), ItemRx = new(@"^(\s*)([-*+]|\d{1,3}[.)])\s+(.*)$"),
        RuleRx = new(@"^\s*([-*_])(\s*\1){2,}\s*$"), QuoteRx = new(@"^\s*>\s?(.*)$");

    internal static List<Block> Parse(string text)
    {
        var lines = text.Replace("\r\n", "\n").Split('\n');
        var blocks = new List<Block>();
        for (int i = 0; i < lines.Length;)
        {
            var line = lines[i];
            var trimmed = line.TrimStart();
            if (trimmed.StartsWith("```") || trimmed.StartsWith("~~~"))
            {
                var fence = trimmed[..3];
                var lang = trimmed[3..].Trim();
                var code = new StringBuilder();
                bool closed = false;
                for (i++; i < lines.Length; i++)
                {
                    if (lines[i].TrimStart().StartsWith(fence)) { closed = true; i++; break; }
                    if (code.Length > 0) code.Append('\n');
                    code.Append(lines[i]);
                }
                blocks.Add(new Block(BlockKind.Code, code.ToString(), Lang: lang, Open: !closed));
                continue;
            }
            if (trimmed.Length == 0) { i++; continue; }
            if (HeadingRx.Match(trimmed) is { Success: true } h) { blocks.Add(new Block(BlockKind.Heading, h.Groups[2].Value.TrimEnd('#', ' '), h.Groups[1].Length)); i++; continue; }
            if (RuleRx.IsMatch(line)) { blocks.Add(new Block(BlockKind.Rule, "")); i++; continue; }
            if (trimmed.StartsWith('|'))
            {
                var rows = new List<string>();
                while (i < lines.Length && lines[i].TrimStart().StartsWith('|')) rows.Add(lines[i++].Trim());
                blocks.Add(new Block(BlockKind.Table, string.Join("\n", rows)));
                continue;
            }
            if (QuoteRx.Match(line) is { Success: true })
            {
                var q = new List<string>();
                while (i < lines.Length && QuoteRx.Match(lines[i]) is { Success: true } qm) { q.Add(qm.Groups[1].Value); i++; }
                blocks.Add(new Block(BlockKind.Quote, string.Join("\n", q)));
                continue;
            }
            if (ItemRx.Match(line) is { Success: true } it)
            {
                var item = new StringBuilder(it.Groups[3].Value);
                int indent = it.Groups[1].Value.Replace("\t", "    ").Length;
                // Lines indented under the item continue it.
                for (i++; i < lines.Length && lines[i].Length > 0 && char.IsWhiteSpace(lines[i][0]) && !ItemRx.IsMatch(lines[i]); i++) item.Append(' ').Append(lines[i].Trim());
                var marker = it.Groups[2].Value;
                blocks.Add(new Block(BlockKind.Item, item.ToString(), Math.Min(indent / 2, 4), char.IsDigit(marker[0]) ? marker.TrimEnd(')', '.') + "." : "•"));
                continue;
            }
            var para = new StringBuilder(line.Trim());
            for (i++; i < lines.Length; i++)
            {
                var l = lines[i];
                var t = l.TrimStart();
                if (t.Length == 0 || t.StartsWith("```") || t.StartsWith("~~~") || t.StartsWith('|') || HeadingRx.IsMatch(t) || ItemRx.IsMatch(l) || QuoteRx.IsMatch(l) || RuleRx.IsMatch(l)) break;
                para.Append('\n').Append(l.Trim());
            }
            blocks.Add(new Block(BlockKind.Para, para.ToString()));
        }
        return blocks;
    }

}
