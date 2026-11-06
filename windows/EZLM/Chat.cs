using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Documents;
using Microsoft.UI.Xaml.Input;
using Windows.System;
using static EZLM.Ui;

namespace EZLM;

/// Chat: talk to your model right here, terminal style. The signal across the
/// top shows what the model is doing; answers stream in as formatted text
/// with the key filled in for you, and nothing leaves the computer.
public sealed partial class Shell
{
    sealed class ChatMsg
    {
        public bool User;
        public readonly StringBuilder Text = new(), Thinking = new();
        public List<(string Name, string Text)> Files = new();
        public List<string> Remembered = new(); // facts noticed after this answer
        public string Model = "";               // who wrote the answer (models can be switched mid-chat)
        public StackPanel? Notes;
        public string Error = "";
        public bool Done, ShowThinking, Stopped;
        public bool? Cursor; // whether the shown text ends with the cursor
        public bool NoThink;  // asked to answer without thinking
        public int Left;      // older messages left out so the chat fit the model
        public bool Cut;      // the answer ran into the model's limit
        public Button? SkipThink;
        public DateTime Started = DateTime.Now, FirstToken, ThinkStart, ThinkEnd, Ended;
        public int Shown, Chunks, Tokens; // characters revealed so far; pieces and tokens received
        public double Rate;
        // This message's views; rebuilt whenever the page is.
        public MdView? Md;
        public TextBlock? Head, ThinkHead, ThinkBody, Stats;
        public Button? ThinkToggle;
        public StackPanel? Foot;
        public Border? Bar;
    }

    readonly List<ChatMsg> chat = new();
    readonly WaveState wave0 = new();
    string chatId = ChatStore.NewId();
    readonly List<(string Name, string Text)> chatFiles = new(); // attached, waiting to be sent
    string? chatBriefing;           // what's remembered, fixed for this chat so the model's cache keeps working
    CancellationTokenSource? memCts; // the quiet look for new facts, cancelled the moment you type
    int memFrom;                    // first message the next look covers
    bool memWorking;
    Button? chatMemBtn, chatThinkBtn;
    bool chatSkipThink; // the running answer was stopped to ask again without thinking
    DateTime chatSentAt;
    StackPanel? chatChips, chatPalette;

    bool chatRestored;
    Button? chatRetry, chatContinue;
    string chatDraft = "";
    CancellationTokenSource? chatCts;
    SignalWave? wave;
    ScrollViewer? chatScroll;
    StackPanel? chatList;
    TextBox? chatBox;
    Button? chatSend;
    Border? chatNotice, chatInputFrame;
    TextBlock? chatState, chatSpeed, chatCtxText;
    Border? chatDot;
    BlockBar? chatCtxBar;
    bool chatFollow = true;
    int chatCtxUsed, chatFrame;
    bool chatInline;          // this chat's model has no place for notes: they go in the first question
    DateTime memEndedAt;      // when the last quiet memory check finished
    double chatShare = 1;     // how much of the room this chat's model took (below 1 after a squeeze)
    DateTime chatErrorAt;
    static readonly HttpClient chatHttp = new() { Timeout = Timeout.InfiniteTimeSpan };
    static readonly string[] Braille = { "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏" };

    static readonly string[] ChatIdeas =
    {
        "Explain how vaccines work, in simple words.",
        "Write a friendly email asking my landlord to fix the heating.",
        "Plan a 3-day vegetarian menu with a shopping list.",
    };

    string ModelLabel => status?.Config?.ModelName ?? "your model";

    UIElement ChatPane()
    {
        // The first time Chat opens, pick up the last conversation (from the past week).
        if (!chatRestored)
        {
            chatRestored = true;
            if (chat.Count == 0 && ChatStore.Recent().FirstOrDefault() is { } last && DateTime.Now - last.Updated < TimeSpan.FromDays(7)) Restore(last);
        }
        var g = new Grid { Margin = new Thickness(30, 18, 30, 16) };
        g.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        g.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        g.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });

        // The signal, and the model's state right under it.
        wave = new SignalWave(wave0) { MoodOf = ChatMood, RateOf = ChatRate };
        wave.Frame += ChatTick;
        chatDot = new Border { Width = 7, Height = 7, CornerRadius = new CornerRadius(4), VerticalAlignment = VerticalAlignment.Center };
        chatState = Txt("", 11, Dim, false, false);
        chatSpeed = Txt("", 11, Faint, false, false);
        chatCtxBar = new BlockBar(8) { Width = 84, VerticalAlignment = VerticalAlignment.Center };
        chatCtxText = Txt("", 11, Faint, false, false);
        var line = new Grid { Margin = new Thickness(0, 6, 0, 0), ColumnSpacing = 12 };
        line.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        line.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        var ctxRow = Row(6, Txt("chat size", 11, Faint, false, false), chatCtxBar, chatCtxText).Also(r => r.VerticalAlignment = VerticalAlignment.Center);
        line.Children.Add(Row(14, Row(6, chatDot, chatState), ctxRow, chatSpeed));
        // A small screen (a 1366x768 laptop) has no room for all of this line: the
        // speed (also in the sidebar) makes way first, then the chat size.
        line.SizeChanged += (_, e) =>
        {
            chatSpeed.Visibility = e.NewSize.Width >= 1000 ? Visibility.Visible : Visibility.Collapsed;
            ctxRow.Visibility = e.NewSize.Width >= 900 ? Visibility.Visible : Visibility.Collapsed;
        };
        chatThinkBtn = Btn("", Kind.Small, () => { Memory.Think = !Memory.Think; UpdateChatInput(); });
        chatThinkBtn.BorderThickness = new Thickness(0); chatThinkBtn.Padding = new Thickness(6, 2, 6, 2);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(chatThinkBtn, "Thinking: let the model think before answering");
        chatMemBtn = Btn("", Kind.Small, () => _ = ShowMemory());
        chatMemBtn.BorderThickness = new Thickness(0); chatMemBtn.Padding = new Thickness(6, 2, 6, 2);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(chatMemBtn, "Memory: what the chat remembers about you");
        var models = Btn("Model ▾", Kind.Small, () => { });
        models.Click += (_, _) => _ = ShowModels(models);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(models, "Switch model");
        var history = Btn("History", Kind.Small, () => { });
        history.Click += (_, _) => ShowHistory(history);
        var acts = Row(6,
            chatThinkBtn,
            chatMemBtn,
            models,
            history,
            Btn("New chat", Kind.Small, NewChat).Also(b => { b.IsEnabled = chat.Count > 0; Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(b, "New chat"); }));
        Grid.SetColumn(acts, 1);
        line.Children.Add(acts);
        var top = Col(0, wave, line, Hairline().Also(h => h.Margin = new Thickness(0, 12, 0, 0)));
        g.Children.Add(top);

        chatList = Col(26);
        chatList.Padding = new Thickness(0, 22, 22, 18);
        chatList.MaxWidth = 900; chatList.HorizontalAlignment = HorizontalAlignment.Left;
        UIElement center;
        if (chat.Count == 0)
        {
            center = Welcome();
        }
        else
        {
            foreach (var m in chat) chatList.Children.Add(MsgView(m));
            center = chatList;
        }
        chatScroll = new ScrollViewer { Content = center, VerticalScrollBarVisibility = ScrollBarVisibility.Auto };
        // Follow new words unless the reader scrolled up to look at something.
        chatScroll.ViewChanged += (s, _) => { var sv = (ScrollViewer)s!; chatFollow = sv.VerticalOffset >= sv.ScrollableHeight - 48; };
        Grid.SetRow(chatScroll, 1); g.Children.Add(chatScroll);

        g.Children.Add(ChatInput().Also(i => Grid.SetRow((FrameworkElement)i, 2)));
        // Typing while a button has focus (after Model, History, memory...) goes
        // into the message box instead of nowhere. Spaces and Enter stay with the
        // button, which they press.
        g.CharacterReceived += (_, e) =>
        {
            if (chatBox is not { IsEnabled: true } box || char.IsControl(e.Character) || char.IsWhiteSpace(e.Character)) return;
            if (FocusManager.GetFocusedElement(g.XamlRoot) is TextBox or PasswordBox) return;
            box.Focus(FocusState.Keyboard);
            // With an input method (Chinese, Japanese, Korean) the key it should have
            // composed arrives as a plain letter: just move to the box, and the next
            // keys compose there as usual.
            if (Windows.Globalization.Language.CurrentInputMethodLanguageTag is var tag && (tag.StartsWith("zh") || tag.StartsWith("ja") || tag.StartsWith("ko"))) { e.Handled = true; return; }
            box.Text += e.Character;
            box.SelectionStart = box.Text.Length;
            e.Handled = true;
        };
        // Drop text files anywhere on the page to ask about them.
        g.AllowDrop = true;
        g.Background = B(Colors.Transparent); // so drops land on empty space too
        g.AddHandler(UIElement.DragOverEvent, new DragEventHandler((_, e) =>
        {
            if (!e.DataView.Contains(Windows.ApplicationModel.DataTransfer.StandardDataFormats.StorageItems)) return;
            e.AcceptedOperation = Windows.ApplicationModel.DataTransfer.DataPackageOperation.Copy;
            e.DragUIOverride.Caption = "Attach to your message";
        }), true);
        g.AddHandler(UIElement.DropEvent, new DragEventHandler(async (_, e) =>
        {
            if (!e.DataView.Contains(Windows.ApplicationModel.DataTransfer.StandardDataFormats.StorageItems)) return;
            e.Handled = true;
            try { Attach((await e.DataView.GetStorageItemsAsync()).Select(i => i.Path)); } catch (Exception ex) { Log(ex); }
        }), true);
        UpdateChatInput();
        ScrollChatToEnd(true);
        return g;
    }

    /// An empty chat: what to do, and three things to try.
    UIElement Welcome()
    {
        var ideas = Col(2);
        foreach (var idea in ChatIdeas)
        {
            var b = Btn(idea, Kind.Small, () => _ = SendChat(idea, fromBox: false));
            b.BorderThickness = new Thickness(0);
            b.Padding = new Thickness(8, 5, 8, 5);
            var t = new TextBlock { FontFamily = Mono, FontSize = 13 };
            t.Inlines.Add(new Run { Text = "❯ ", Foreground = B(Signal) });
            t.Inlines.Add(new Run { Text = idea, Foreground = B(Dim) });
            b.Content = t;
            ideas.Children.Add(b);
        }
        // The model's name in big half-block letters, lilac fading to white.
        var title = new TextBlock
        {
            FontFamily = Mono, FontSize = 15, LineHeight = 15, LineStackingStrategy = LineStackingStrategy.BlockLineHeight,
            TextWrapping = TextWrapping.NoWrap, Text = string.Join("\n", BigType.Lines(BigType.Fit(ModelLabel))),
        };
        var fade = new Microsoft.UI.Xaml.Media.LinearGradientBrush { StartPoint = new Windows.Foundation.Point(0, 0), EndPoint = new Windows.Foundation.Point(1, 1) };
        fade.GradientStops.Add(new Microsoft.UI.Xaml.Media.GradientStop { Offset = 0, Color = Signal });
        fade.GradientStops.Add(new Microsoft.UI.Xaml.Media.GradientStop { Offset = 1, Color = C(0xf2eeff) });
        title.Foreground = fade;
        return Col(16,
            title,
            Txt($"Ask {ModelLabel} anything. It runs on this computer, so what you type stays here.", 12, Dim),
            Txt("It remembers what you tell it about yourself (only here; see ✦ memory). Type / for commands.", 12, Faint),
            Txt("Try one of these, or write below:", 12, Faint),
            ideas).Also(c => { c.VerticalAlignment = VerticalAlignment.Center; c.HorizontalAlignment = HorizontalAlignment.Left; c.Margin = new Thickness(4, 40, 0, 40); c.MaxWidth = 720; });
    }

    UIElement ChatInput()
    {
        chatBox = new TextBox
        {
            Text = chatDraft, AcceptsReturn = true, TextWrapping = TextWrapping.Wrap, MaxHeight = 170, MinHeight = 22, FontFamily = Mono, FontSize = 13.5,
            BorderThickness = new Thickness(0), Padding = new Thickness(0, 4, 0, 4), PlaceholderText = $"Message {ModelLabel}",
        };
        // No box inside the box: the frame around the prompt is the only border.
        var none = B(Colors.Transparent);
        foreach (var k in new[] { "TextControlBackground", "TextControlBackgroundPointerOver", "TextControlBackgroundFocused", "TextControlBackgroundDisabled",
                                  "TextControlBorderBrush", "TextControlBorderBrushPointerOver", "TextControlBorderBrushFocused", "TextControlBorderBrushDisabled" })
            chatBox.Resources[k] = none;
        foreach (var k in new[] { "TextControlForeground", "TextControlForegroundPointerOver", "TextControlForegroundFocused" }) chatBox.Resources[k] = B(Ink);
        foreach (var k in new[] { "TextControlPlaceholderForeground", "TextControlPlaceholderForegroundPointerOver", "TextControlPlaceholderForegroundFocused" }) chatBox.Resources[k] = B(Faint);
        chatBox.Resources["TextControlSelectionHighlightColor"] = B(C(0x4a3f80));
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(chatBox, "Message");
        chatBox.TextChanged += (_, _) => { chatDraft = chatBox.Text; if (chatDraft.Length > 0) memCts?.Cancel(); RenderPalette(); };
        chatBox.PreviewKeyDown += OnChatKey;

        // Send turns into Stop; a double click mustn't stop the answer it just started.
        chatSend = Btn("Send  ↵", Kind.Primary, () => { if (chatCts != null) { if ((DateTime.Now - chatSentAt).TotalMilliseconds > 600) chatCts.Cancel(); } else _ = SendChat(chatBox?.Text ?? ""); });
        chatSend.Padding = new Thickness(16, 6, 16, 6);
        chatSend.VerticalAlignment = VerticalAlignment.Bottom;

        var attach = Btn("+ file", Kind.Small, () => _ = PickFiles());
        attach.VerticalAlignment = VerticalAlignment.Bottom; attach.Margin = new Thickness(0, 0, 0, 3);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(attach, "Attach a text file");
        var prompt = new Grid { ColumnSpacing = 12 };
        prompt.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        prompt.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        prompt.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        prompt.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        prompt.Children.Add(Txt("❯", 15, Signal, true, false).Also(t => t.Margin = new Thickness(0, 5, 0, 0)));
        Grid.SetColumn(chatBox, 1); prompt.Children.Add(chatBox);
        Grid.SetColumn(attach, 2); prompt.Children.Add(attach);
        Grid.SetColumn(chatSend, 3); prompt.Children.Add(chatSend);
        chatInputFrame = new Border { BorderBrush = B(Rule), BorderThickness = new Thickness(1), Padding = new Thickness(14, 8, 8, 8), Background = B(EZLM.Ui.Panel), Child = prompt };
        chatBox.GotFocus += (_, _) => { if (chatInputFrame != null) chatInputFrame.BorderBrush = B(C(0x5a4f8c)); };
        chatBox.LostFocus += (_, _) => { if (chatInputFrame != null) chatInputFrame.BorderBrush = B(Rule); };

        var hints = new TextBlock { FontFamily = Mono, FontSize = 11, Margin = new Thickness(2, 8, 0, 0) };
        foreach (var (key, what) in new[] { ("enter", "send"), ("shift+enter", "new line"), ("esc", "stop"), ("ctrl+n", "new chat"), ("↑", "last message"), ("/remember", "save a fact") })
        {
            hints.Inlines.Add(new Run { Text = key, Foreground = B(Dim) });
            hints.Inlines.Add(new Run { Text = " " + what + "     ", Foreground = B(Faint) });
        }
        chatNotice = new Border { Margin = new Thickness(0, 0, 0, 10) };
        chatNoticeKey = "-";
        if (ServerState == 2) chatBox.Loaded += (_, _) => chatBox?.Focus(FocusState.Programmatic);
        chatChips = Row(8);
        chatChips.Margin = new Thickness(0, 0, 0, 8);
        RenderChips();
        chatPalette = Col(2);
        chatPalette.Margin = new Thickness(0, 0, 0, 8);
        RenderPalette();
        return Col(0, chatNotice, chatPalette, chatChips, chatInputFrame, hints).Also(c => c.Margin = new Thickness(0, 10, 0, 0));
    }

    void OnChatKey(object sender, KeyRoutedEventArgs e)
    {
        if ((int)e.Key == 229) return; // an input method (Chinese, Japanese, Korean) is using this key to build a word
        var ctrl = Microsoft.UI.Input.InputKeyboardSource.GetKeyStateForCurrentThread(VirtualKey.Control).HasFlag(Windows.UI.Core.CoreVirtualKeyStates.Down);
        var shift = Microsoft.UI.Input.InputKeyboardSource.GetKeyStateForCurrentThread(VirtualKey.Shift).HasFlag(Windows.UI.Core.CoreVirtualKeyStates.Down);
        if (e.Key == VirtualKey.Enter && !shift) { e.Handled = true; if (chatCts == null) _ = SendChat(chatBox?.Text ?? ""); }
        else if (e.Key == VirtualKey.Escape && chatCts != null) { e.Handled = true; chatCts.Cancel(); }
        else if (e.Key == VirtualKey.N && ctrl) { e.Handled = true; NewChat(); }
        else if (e.Key == VirtualKey.Tab && chatBox is { } tb && ChatLogic.Matches(tb.Text) is { Count: > 0 } found)
        {
            // Like a shell: Tab finishes the command.
            e.Handled = true;
            tb.Text = found[0].Cmd + (found[0].Arg.Length > 0 ? " " : "");
            tb.SelectionStart = tb.Text.Length;
        }
        else if (e.Key == VirtualKey.Up && chatBox is { Text.Length: 0 } box && chat.LastOrDefault(m => m.User && m.Text.ToString() != ChatLogic.ContinueText) is { } last)
        {
            // Like a terminal: up brings back what you sent last (with its files), to fix and resend.
            e.Handled = true;
            box.Text = last.Text.ToString();
            box.SelectionStart = box.Text.Length;
            if (chatFiles.Count == 0 && last.Files.Count > 0) { chatFiles.AddRange(last.Files); RenderChips(); }
        }
    }

    /// Esc and Ctrl+N work in Chat even when the message box isn't focused.
    bool ChatKey(KeyRoutedEventArgs e)
    {
        if (screen != Screen.Main || section != "Chat") return false;
        var ctrl = Microsoft.UI.Input.InputKeyboardSource.GetKeyStateForCurrentThread(VirtualKey.Control).HasFlag(Windows.UI.Core.CoreVirtualKeyStates.Down);
        if (e.Key == VirtualKey.Escape && chatCts != null) { chatCts.Cancel(); return e.Handled = true; }
        if (e.Key == VirtualKey.N && ctrl) { NewChat(); return e.Handled = true; }
        return false;
    }

    UIElement MsgView(ChatMsg m)
    {
        var col = Col(8);
        m.Head = Txt("", 11, m.User ? Dim : Signal, false, false);
        col.Children.Add(m.Head);
        if (m.User)
        {
            if (m.Files.Count > 0) col.Children.Add(Row(8, m.Files.Select(f => Chip(f.Name, f.Text, null)).ToArray()));
            if (m.Text.Length > 0) col.Children.Add(new TextBlock { Text = m.Text.ToString(), FontFamily = Mono, FontSize = 13, TextWrapping = TextWrapping.Wrap, IsTextSelectionEnabled = true, Foreground = B(Ink), LineHeight = 20 });
        }
        else
        {
            m.ThinkHead = Txt("", 11, Gold, false, false);
            m.ThinkToggle = Btn("", Kind.Small, () => { m.ShowThinking = !m.ShowThinking; UpdateThinking(m); });
            m.ThinkToggle.BorderThickness = new Thickness(0); m.ThinkToggle.Padding = new Thickness(0, 2, 6, 2); m.ThinkToggle.HorizontalAlignment = HorizontalAlignment.Left;
            m.ThinkBody = new TextBlock { FontFamily = Mono, FontSize = 12, TextWrapping = TextWrapping.Wrap, IsTextSelectionEnabled = true, Foreground = B(C(0x8a8a8a)), LineHeight = 18 };
            var think = new Grid { ColumnSpacing = 12 };
            think.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1) });
            think.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
            think.Children.Add(new Border { Background = B(C(0x4d4426)) });
            Grid.SetColumn(m.ThinkBody, 1); think.Children.Add(m.ThinkBody);
            think.Visibility = Visibility.Collapsed;
            m.ThinkBody.Tag = think;
            col.Children.Add(m.ThinkHead);
            col.Children.Add(m.ThinkToggle);
            col.Children.Add(think);
            m.SkipThink = Btn("Answer without thinking", Kind.Small, () => { chatSkipThink = true; chatCts?.Cancel(); });
            m.SkipThink.HorizontalAlignment = HorizontalAlignment.Left;
            m.SkipThink.Visibility = Visibility.Collapsed;
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(m.SkipThink, "Answer without thinking: stop and ask again for a direct answer");
            col.Children.Add(m.SkipThink);
            m.Md = new MdView();
            col.Children.Add(m.Md.Root);
            m.Stats = Txt("", 11, Faint, false, false);
            m.Foot = Row(12, m.Stats);
            col.Children.Add(m.Foot);
            m.Notes = Col(4);
            col.Children.Add(m.Notes);
            RenderNotes(m);
        }
        m.Bar = new Border { Background = B(m.User ? Faint : Signal), Width = 2 };
        var g = new Grid { ColumnSpacing = 16 };
        g.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        g.Children.Add(m.Bar);
        Grid.SetColumn(col, 1); g.Children.Add(col);
        if (m.Done) m.Shown = m.Text.Length;
        RefreshMsg(m, true);
        return g;
    }

    /// Brings a message's views up to date; full re-renders the text too.
    void RefreshMsg(ChatMsg m, bool full)
    {
        if (m.Head == null) return;
        if (m.User) { m.Head.Text = "you"; return; }
        var secs = ((m.Done ? m.Ended : DateTime.Now) - m.Started).TotalSeconds;
        var spin = Braille[chatFrame / 2 % Braille.Length];
        string state = m.Error != "" ? "didn't finish" : m.Stopped ? "stopped" : m.Done ? $"{secs:0.0}s" + (m.NoThink ? "  ·  without thinking" : "")
            : m.Text.Length > 0 ? $"writing {spin}"
            : m.Thinking.Length > 0 ? $"thinking {spin}"
            : $"reading {spin}" + (live is { PromptTotal: > 0 } l ? $"  {ChatLogic.Short(l.PromptDone)} / {ChatLogic.Short(l.PromptTotal)} tokens" : "");
        m.Head.Text = $"◆ {(m.Model != "" ? m.Model : ModelLabel)}  ·  {state}";
        if (m.Bar != null) m.Bar.Background = B(m.Error != "" ? Bad : m.Done ? C(0x5a4f8c) : Signal);
        UpdateThinking(m);
        bool cursor = !m.Done && (m.Text.Length > 0 || m.Thinking.Length == 0); // none while it thinks
        if (m.Cursor != cursor) full = true;
        if (full && m.Md != null)
        {
            m.Cursor = cursor;
            var text = m.Text.ToString(0, Math.Min(m.Shown, m.Text.Length));
            m.Md.Update(text, cursor);
            if (m.Error != "") m.Md.Root.Children.Add(new TextBlock { Text = m.Error, FontFamily = Mono, FontSize = 13, TextWrapping = TextWrapping.Wrap, Foreground = B(Bad), IsTextSelectionEnabled = true });
        }
        if (m.Stats != null)
            m.Stats.Text = !m.Done ? "" : string.Join("   ", new[] { m.Rate > 0 ? $"{m.Rate:0} tokens/s" : "", m.Tokens > 0 ? $"{m.Tokens} tokens" : "", m.Stopped ? "stopped by you" : "", m.Cut ? "stopped at the model's limit" : "", m.Left > 0 && m.Error == "" ? "older messages left out" : "" }.Where(x => x != ""));
        if (m.Foot != null && m.Done && m.Foot.Children.Count == 1 && (m.Text.Length > 0 || m.Error != "" || m.Stopped))
        {
            if (m.Text.Length > 0)
            {
                m.Foot.Children.Insert(0, CopyBtn(m.Text.ToString(), "Copy answer"));
                Button? listen = null;
                listen = Btn("Listen", Kind.Small, () => _ = Speak(m, listen!));
                Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(listen, "Listen: read this answer aloud");
                m.Foot.Children.Insert(1, listen);
            }
            if (chat.LastOrDefault() == m)
            {
                if (chatRetry?.Parent is Panel old) old.Children.Remove(chatRetry);
                chatRetry = Btn("Retry", Kind.Small, RetryLast);
                Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(chatRetry, "Retry: ask for a new answer");
                m.Foot.Children.Insert(m.Text.Length > 0 ? 2 : 0, chatRetry);
                if (chatContinue?.Parent is Panel oldc) oldc.Children.Remove(chatContinue);
                chatContinue = null;
                if (m.Text.Length == 0 && m.Thinking.Length > 0 && m.Error != "")
                {
                    // It thought until it ran out of room and never answered: one click for a straight answer.
                    chatContinue = Btn("Answer without thinking", Kind.Small, () => RetryLast(false));
                    Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(chatContinue, "Answer without thinking: ask again for a straight answer");
                    m.Foot.Children.Insert(1, chatContinue);
                }
                else if ((m.Cut || m.Stopped) && m.Text.Length > 0)
                {
                    // The answer ran into the model's limit, or was stopped to read: let it carry on from there.
                    chatContinue = Btn("Continue", Kind.Small, () => _ = SendChat(ChatLogic.ContinueText, think: false, files: new() { ChatLogic.AnswerEnd(m.Text.ToString()) }, fromBox: false)); // carrying on needs no thinking
                    Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(chatContinue, "Continue: ask the model to go on where the answer stopped");
                    m.Foot.Children.Insert(3, chatContinue);
                }
            }
        }
    }

    void UpdateThinking(ChatMsg m)
    {
        if (m.ThinkHead == null || m.ThinkToggle == null || m.ThinkBody?.Tag is not Grid box) return;
        bool any = m.Thinking.Length > 0, live = any && m.Text.Length == 0 && !m.Done;
        m.ThinkHead.Visibility = live ? Visibility.Visible : Visibility.Collapsed;
        m.ThinkToggle.Visibility = any && !live ? Visibility.Visible : Visibility.Collapsed;
        if (live)
        {
            // While it thinks: the latest few lines of it, dimmed, like a log scrolling by.
            var t = m.Thinking.ToString();
            var tail = t.Length > 360 ? "…" + t[^360..] : t;
            var secs = (DateTime.Now - m.ThinkStart).TotalSeconds;
            m.ThinkHead.Text = $"∴ thinking  {secs:0}s";
            // Long thinking on a slow computer: offer the way straight to an answer.
            if (m.SkipThink != null) m.SkipThink.Visibility = secs > 40 ? Visibility.Visible : Visibility.Collapsed;
            m.ThinkBody.Text = tail.Trim();
            box.Visibility = Visibility.Visible;
        }
        else if (any)
        {
            if (m.SkipThink != null) m.SkipThink.Visibility = Visibility.Collapsed;
            var end = m.ThinkEnd == default ? m.Ended : m.ThinkEnd;
            ((TextBlock)m.ThinkToggle.Content).Text = $"∴ thought for {Math.Max(1, (end - m.ThinkStart).TotalSeconds):0}s  {(m.ShowThinking ? "▾" : "▸")}";
            ((TextBlock)m.ThinkToggle.Content).Foreground = B(C(0x9c8a52));
            box.Visibility = m.ShowThinking ? Visibility.Visible : Visibility.Collapsed;
            if (m.ShowThinking) m.ThinkBody.Text = m.Thinking.ToString().Trim();
        }
        else box.Visibility = Visibility.Collapsed;
    }

    /// 30 times a second while Chat is on screen: words appear smoothly
    /// (catching up with what arrived), and the live parts tick.
    void ChatTick()
    {
        chatFrame++;
        if (chatFrame % 2 != 0) return; // 15 updates a second is smooth and leaves room for typing
        if (chat.LastOrDefault() is not { User: false, Done: false } m) return;
        bool grew = false;
        if (m.Shown < m.Text.Length)
        {
            m.Shown = Math.Min(m.Text.Length, m.Shown + Math.Max(3, (m.Text.Length - m.Shown + 2) / 3));
            if (m.Shown < m.Text.Length && char.IsHighSurrogate(m.Text[m.Shown - 1])) m.Shown++;
            grew = true;
        }
        RefreshMsg(m, grew);
        if (grew) ScrollChatToEnd();
    }

    Mood ChatMood()
    {
        var st = ServerState;
        if (st == 0) return Mood.Off;
        if (st == 1) return Mood.Loading;
        if (chat.LastOrDefault() is { User: false, Done: false } m) return m.Text.Length > 0 ? Mood.Writing : m.Thinking.Length > 0 ? Mood.Thinking : Mood.Reading;
        if ((DateTime.Now - chatErrorAt).TotalSeconds < 2.5) return Mood.Error;
        // Another app (OpenCode, Hermes, a script) using the model shows here too.
        return live?.Phase switch { "writing" => Mood.Writing, "reading" => Mood.Reading, _ => Mood.Idle };
    }

    double ChatRate()
    {
        if (chat.LastOrDefault() is { User: false, Done: false, Chunks: > 4 } m && m.FirstToken != default)
            return m.Chunks / Math.Max(0.3, (DateTime.Now - m.FirstToken).TotalSeconds);
        return live?.TokPerSec ?? 0;
    }

    /// The state line under the signal, and whether you can send right now.
    void UpdateChatInput()
    {
        if (chatSend == null || chatNotice == null || chatBox == null) return;
        var st = ServerState;
        ((TextBlock)chatSend.Content).Text = chatCts != null ? "Stop  esc" : "Send  ↵";
        chatSend.IsEnabled = chatCts != null || st == 2;
        if (chatDot != null && chatState != null)
        {
            chatDot.Background = B(st == 2 ? Ok : st == 1 ? Gold : Bad);
            // The live reading lags a moment behind: just after our own answer, it's still ours.
            double sinceAnswer = chat.LastOrDefault(m => !m.User) is { Done: true } lastReply ? (DateTime.Now - lastReply.Ended).TotalSeconds : 99;
            bool justAnswered = sinceAnswer is >= 0 and < 4 || (DateTime.Now - memEndedAt).TotalSeconds < 4;
            chatState.Text = st == 2 ? (memWorking ? "noting what to remember" : live?.Phase is "writing" or "reading" && chatCts == null && !justAnswered ? "busy with another app" : "live") : st == 1 ? "loading" : status?.Deployed == true ? "stopped" : "not set up";
        }
        var max = status?.Config?.Ctx ?? 0;
        if (chatCtxBar != null && chatCtxText != null)
        {
            double f = max > 0 ? Math.Min(1, chatCtxUsed / (double)max) : 0;
            chatCtxBar.SetFill(f, f > 0.9 ? Bad : f > 0.75 ? Gold : Signal);
            chatCtxText.Text = max > 0 ? $"{ChatLogic.Short(chatCtxUsed)} / {ChatLogic.Short(max)}" : "";
        }
        if (chatThinkBtn != null) ((TextBlock)chatThinkBtn.Content).Text = Memory.Think ? "∴ thinking on" : "∴ thinking off";
        if (chatMemBtn != null)
        {
            var n = Memory.Facts().Count;
            ((TextBlock)chatMemBtn.Content).Text = !Memory.On ? "✦ memory off" : n == 0 ? "✦ memory" : $"✦ remembers {n}";
        }
        if (chatSpeed != null)
        {
            var last = chat.LastOrDefault(m => !m.User && m.Rate > 0);
            chatSpeed.Text = last != null ? $"{last.Rate:0} tokens/s" : "";
        }
        UIElement? notice = st == 2
            ? (max > 0 && chatCtxUsed > max * 0.9 ? Txt("This chat has nearly filled the model's memory, so its oldest messages are left out from here. New chat (ctrl+n) starts fresh.", 12, Gold) : null)
            : st == 1 ? Row(10, new Spinner(Gold), Txt($"{ModelLabel} is loading. You can chat in a moment.", 12, Gold))
            : status?.Deployed == true ? Row(12, Txt("Your model is stopped.", 12, Bad).Also(t => t.VerticalAlignment = VerticalAlignment.Center), Btn("Start server", Kind.Small, () => _ = StartServer()))
            : Row(12, Txt("Set up a model first: pick one in Models.", 12, Dim).Also(t => t.VerticalAlignment = VerticalAlignment.Center), Btn("Open Models", Kind.Small, () => Go("Models")));
        // Only swap the notice when what it says changes (polls arrive every second).
        var key = notice == null ? "" : $"{st}|{status?.Deployed}|{chatCtxUsed > max * 0.9}";
        if (key == chatNoticeKey && chatNotice.Child != null == (notice != null)) return;
        chatNoticeKey = key;
        chatNotice.Child = notice;
        chatNotice.Visibility = notice == null ? Visibility.Collapsed : Visibility.Visible;
    }

    string chatNoticeKey = "";


    void ScrollChatToEnd(bool force = false)
    {
        if (chatScroll == null || !(force || chatFollow)) return;
        var sv = chatScroll;
        dq.TryEnqueue(Microsoft.UI.Dispatching.DispatcherQueuePriority.Low, () => { sv.UpdateLayout(); sv.ChangeView(null, sv.ScrollableHeight, null, true); });
    }

    void NewChat()
    {
        if (chatCts != null) SaveChat(); // keep the turn in progress
        chatCts?.Cancel();
        chat.Clear();
        chatId = ChatStore.NewId();
        chatBriefing = null; chatInline = false; chatShare = 1;
        memCts?.Cancel();
        memFrom = 0;
        chatDraft = "";
        chatCtxUsed = 0;
        if (section == "Chat") ShowSection();
    }

    async Task SendChat(string text, bool? think = null, List<(string Name, string Text)>? files = null, bool fromBox = true)
    {
        text = text.Trim();
        if (fromBox && ChatCommand(text)) return;
        var attached = files ?? (fromBox ? chatFiles.ToList() : new());
        var (c, url) = ChatTarget();
        if ((text == "" && attached.Count == 0) || chatCts != null || c == null || string.IsNullOrEmpty(url) || ServerState != 2) return;
        bool first = chat.Count == 0;
        var user = new ChatMsg { User = true, Done = true, Files = attached };
        user.Text.Append(text);
        if (fromBox) { chatFiles.Clear(); RenderChips(); chatDraft = ""; } // a suggestion or a retry leaves what you're typing alone
        chat.Add(user);
        var reply = new ChatMsg { Model = ModelLabel, NoThink = !(think ?? Memory.Think) };
        chat.Add(reply);
        // Earlier turns go along so it remembers the conversation; a question
        // whose answer failed is left out (many models need user/assistant turns
        // to alternate).
        memCts?.Cancel();
        chatBriefing ??= Memory.Briefing(); // set once, then fixed for the chat
        var history = History(chat.Count - 1, c.Ctx, out var left, chatInline, chatShare); // as this chat's model needed last time
        reply.Left = left;
        chatFollow = true;
        chatSentAt = DateTime.Now;
        SaveChat(); // the question is kept even if EZLM closes before the answer ends
        var cts = chatCts = new CancellationTokenSource();
        bool stopped = false;
        if (section == "Chat")
        {
            if (first || chatList == null) ShowSection();
            else
            {
                if (chatRetry?.Parent is Panel p) p.Children.Remove(chatRetry);
                if (chatContinue?.Parent is Panel pc) pc.Children.Remove(chatContinue);
                chatList.Children.Add(MsgView(user));
                chatList.Children.Add(MsgView(reply));
                if (fromBox && chatBox != null) chatBox.Text = "";
                UpdateChatInput();
                ScrollChatToEnd(true);
            }
        }
        try
        {
            if (Engine.Demo && TestServer == null) { await DemoReply(reply, cts.Token); return; }
            // Without thinking: Qwen-style templates skip the think block; gpt-oss thinks briefly.
            var options = reply.NoThink ? new Dictionary<string, object> { ["chat_template_kwargs"] = new Dictionary<string, object> { ["enable_thinking"] = false }, ["reasoning_effort"] = "low" } : null;
            bool inline = chatInline;
            double share = chatShare;
            int squeezes = 0;
            HttpResponseMessage resp;
            while (!(resp = await PostChat(url, c, history, true, cts.Token, options)).IsSuccessStatusCode)
            {
                var err = await resp.Content.ReadAsStringAsync(cts.Token);
                var code = (int)resp.StatusCode;
                resp.Dispose();
                switch (ChatLogic.AfterRefusal(err, inline, chatBriefing != null, squeezes, history.Count(t => t.Role == "user")))
                {
                    case ChatLogic.Refusal.Inline: inline = true; break;
                    case ChatLogic.Refusal.Squeeze:
                        squeezes++;
                        share = ChatLogic.Squeeze(err, history, c.Ctx, chatBriefing == null ? 0 : ChatLogic.Tokens(chatBriefing), share);
                        break;
                    default: reply.Error = ChatLogic.ChatError(code, err); return;
                }
                history = History(chat.Count - 1, c.Ctx, out left, inline, share);
                reply.Left = Math.Max(reply.Left, left);
            }
            chatInline = inline; chatShare = share; // the memory check asks the same way
            using var _resp = resp;
            using var reader = new StreamReader(await resp.Content.ReadAsStreamAsync(cts.Token));
            while (await reader.ReadLineAsync(cts.Token) is { } line)
            {
                if (!line.StartsWith("data: ")) continue;
                var data = line[6..].Trim();
                if (data == "[DONE]") break;
                using var doc = JsonDocument.Parse(data);
                var root = doc.RootElement;
                if (root.TryGetProperty("error", out _)) { reply.Error = ChatLogic.ChatError(500, data); break; }
                if (root.TryGetProperty("choices", out var choices) && choices.GetArrayLength() > 0 && choices[0].TryGetProperty("delta", out var d))
                {
                    if (d.TryGetProperty("reasoning_content", out var r) && r.ValueKind == JsonValueKind.String && r.GetString() is { Length: > 0 } thought)
                        Think(reply, thought);
                    if (d.TryGetProperty("content", out var ct) && ct.ValueKind == JsonValueKind.String && ct.GetString() is { Length: > 0 } piece)
                        Write(reply, piece);
                    if (choices[0].TryGetProperty("finish_reason", out var fr) && fr.ValueKind == JsonValueKind.String && fr.GetString() == "length") reply.Cut = true;
                }
                if (root.TryGetProperty("timings", out var tm))
                {
                    if (tm.TryGetProperty("predicted_per_second", out var pps) && pps.TryGetDouble(out var tps)) reply.Rate = tps;
                    int n(string k) => tm.TryGetProperty(k, out var v) && v.TryGetInt32(out var i) ? i : 0;
                    reply.Tokens = n("predicted_n");
                    chatCtxUsed = n("prompt_n") + n("cache_n") + reply.Tokens;
                }
            }
            if (reply.Text.Length == 0 && reply.Error == "")
                reply.Error = reply.Thinking.Length > 0 ? "It thought about it but didn't write an answer. Try asking again." : "It didn't answer. Try asking again.";
        }
        catch (OperationCanceledException) { stopped = reply.Stopped = true; }
        catch (HttpRequestException) { reply.Error = "Couldn't reach your model. It may have stopped: check Overview."; }
        catch (IOException) { reply.Error = "Your model stopped in the middle of answering. Check Overview, then ask again."; }
        catch (Exception ex) { Log(ex); reply.Error = "Something went wrong: " + ex.Message; }
        finally
        {
            reply.Done = true;
            reply.Ended = DateTime.Now;
            reply.Shown = reply.Text.Length;
            if (reply.Thinking.Length > 0 && reply.ThinkEnd == default) reply.ThinkEnd = reply.Ended;
            if (reply.Rate == 0 && reply.Chunks > 4 && reply.FirstToken != default) reply.Rate = reply.Chunks / Math.Max(0.3, (reply.Ended - reply.FirstToken).TotalSeconds);
            if (chatCts == cts) chatCts = null;
            cts.Dispose();
            if (reply.Error != "") chatErrorAt = DateTime.Now; else if (!stopped) wave?.Ripple();
            // A long answer that finished while you were elsewhere: the taskbar button says so.
            if (!stopped && (reply.Ended - reply.Started).TotalSeconds > 10) Taskbar.Flash(Hwnd);
            if (chat.Contains(reply)) SaveChat();
            if (reply.Error == "" && !stopped && chat.Contains(reply)) _ = NoticeFacts(reply);
            if (section == "Chat") { RefreshMsg(reply, true); UpdateChatInput(); ScrollChatToEnd(); chatBox?.Focus(FocusState.Programmatic); }
            if (chatSkipThink) { chatSkipThink = false; if (stopped && chat.LastOrDefault() == reply) RetryLast(think: false); }
        }
    }

    void Think(ChatMsg m, string piece)
    {
        if (m.Thinking.Length == 0) m.ThinkStart = DateTime.Now;
        m.Thinking.Append(piece);
        wave?.Pulse(0.02);
    }

    void Write(ChatMsg m, string piece)
    {
        if (m.FirstToken == default) m.FirstToken = DateTime.Now;
        if (m.Thinking.Length > 0 && m.ThinkEnd == default) m.ThinkEnd = DateTime.Now;
        // A small model can leak its thinking tags into the answer; they mean nothing to a reader.
        piece = piece.Replace("</think>", "").Replace("<think>", "");
        if (piece.Length == 0) return;
        // Thinking models often open the answer with blank lines.
        m.Text.Append(m.Text.Length == 0 ? piece.TrimStart() : piece);
        m.Chunks++;
        wave?.Pulse(0.05);
    }

    /// The demo has no model: a short, honest stand-in shows how answers look.
    async Task DemoReply(ChatMsg m, CancellationToken ct)
    {
        await Task.Delay(700, ct);
        foreach (var w in "The person wants to see what an answer looks like here. A short reply with a list, a little code and a table shows it all.".Split(' '))
        {
            Think(m, w + " ");
            await Task.Delay(45, ct);
        }
        var answer = "This is **EZLM's demo**, so there's no real model here. In the real app, your model answers right on this computer.\n\n" +
            "Answers can have:\n\n- **lists** like this one\n- `inline code` and [links](https://github.com/picom-code/ezlm)\n- code with a copy button:\n\n" +
            "```python\n# count the words in a file\nwith open(\"notes.txt\") as f:\n    print(len(f.read().split()))\n```\n\n" +
            "| model | size | speed |\n|---|---|---|\n| Qwen 3.5 9B | 5.6 GB | 62 tok/s |\n| Gemma 4 12B | 7.1 GB | 48 tok/s |\n";
        for (int i = 0; i < answer.Length; i += 3)
        {
            Write(m, answer.Substring(i, Math.Min(3, answer.Length - i)));
            await Task.Delay(16, ct);
        }
        m.Rate = 62; m.Tokens = 140;
        chatCtxUsed += 420;
    }

    /// The messages for the model (ChatLogic.Messages) from this chat, up to
    /// (not including) message `upto`.
    List<(string Role, string Content)> History(int upto, int ctx, out int left, bool inline = false, double share = 1) =>
        ChatLogic.Messages(chat.Select(m => (m.User, m.User ? MessageContent(m) : m.Text.ToString())).ToList(), upto, chatBriefing, ctx, inline, share, out left);

    static async Task<HttpResponseMessage> PostChat(string url, ServerConfig c, List<(string Role, string Content)> msgs, bool stream, CancellationToken ct, object? extra = null)
    {
        var payload = new Dictionary<string, object> { ["model"] = c.ModelId, ["stream"] = stream, ["messages"] = msgs.Select(m => new Dictionary<string, string> { ["role"] = m.Role, ["content"] = m.Content }).ToList() };
        if (extra is Dictionary<string, object> more) foreach (var kv in more) payload[kv.Key] = kv.Value;
        var req = new HttpRequestMessage(HttpMethod.Post, url.TrimEnd('/') + "/chat/completions") { Content = new StringContent(JsonSerializer.Serialize(payload), Encoding.UTF8, "application/json") };
        req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", c.Key);
        return await chatHttp.SendAsync(req, stream ? HttpCompletionOption.ResponseHeadersRead : HttpCompletionOption.ResponseContentRead, ct);
    }

    /// /remember, /forget, /memory, /new: handled here, never sent to the model.
    bool ChatCommand(string text)
    {
        if (!text.StartsWith('/')) return false;
        var sp = text.IndexOf(' ');
        var cmd = (sp < 0 ? text : text[..sp]).ToLowerInvariant();
        var arg = sp < 0 ? "" : text[(sp + 1)..].Trim();
        void Done() { chatDraft = ""; if (chatBox != null) chatBox.Text = ""; UpdateChatInput(); }
        switch (cmd)
        {
            case "/remember":
                if (arg == "") { Toast("Write what to remember after /remember, like: /remember I'm vegetarian", Gold); return true; }
                var added = Memory.Add(arg);
                Toast(added.Ok ? $"Remembered: {added.Fact}" : added.Why, added.Ok ? Ok : Gold);
                Done();
                return true;
            case "/forget":
                if (arg == "") { _ = ShowMemory(); Done(); return true; }
                var n = Memory.Forget(arg);
                Toast(n > 0 ? $"Forgot {n} thing{(n == 1 ? "" : "s")} about \"{arg}\"." : $"Nothing remembered mentions \"{arg}\".", n > 0 ? Ok : Gold);
                Done();
                return true;
            case "/memory": Done(); _ = ShowMemory(); return true;
            case "/new": Done(); NewChat(); return true;
            case "/help" or "/":
                Toast("ChatLogic.Commands: /remember <fact>   /forget <words>   /memory   /new", Ink);
                return true;
        }
        return false; // not a command: an ordinary message that starts with /
    }

    /// After an answer, once the chat has gone quiet for a few seconds, asks
    /// the model (briefly, without thinking) for lasting facts the person
    /// shared, and keeps the new ones. Typing or sending cancels it at once,
    /// so it never slows the conversation down.
    async Task NoticeFacts(ChatMsg reply)
    {
        var (c, url) = ChatTarget();
        if (!Memory.On || !Memory.Auto || (Engine.Demo && TestServer == null) || c == null || url == null) return;
        int upto = chat.IndexOf(reply) + 1;
        if (!chat.Skip(memFrom).Take(upto - memFrom).Any(m => m.User && Memory.Personal(m.Text.ToString()))) return;
        var cts = memCts = new CancellationTokenSource();
        try
        {
            await Task.Delay(8000, cts.Token); // quiet first
            if (chatCts != null || ServerState != 2 || chatDraft.Length > 0) return;
            memWorking = true;
            UpdateChatInput();
            var msgs = History(upto, c.Ctx, out _, chatInline, chatShare);
            msgs.Add(("user", Memory.ExtractPrompt));
            // Thinking off both ways (as in Chat): gpt-oss always reasons, and would spend the whole answer on it.
            var extra = new Dictionary<string, object> { ["max_tokens"] = 220, ["temperature"] = 0.2, ["chat_template_kwargs"] = new Dictionary<string, object> { ["enable_thinking"] = false }, ["reasoning_effort"] = "low" };
            using var resp = await PostChat(url, c, msgs, false, cts.Token, extra);
            if (!resp.IsSuccessStatusCode) return;
            using var doc = JsonDocument.Parse(await resp.Content.ReadAsStringAsync(cts.Token));
            var content = doc.RootElement.GetProperty("choices")[0].GetProperty("message").TryGetProperty("content", out var ct) ? ct.GetString() ?? "" : "";
            memFrom = upto;
            foreach (var fact in Memory.ParseFacts(content))
                if (Memory.Add(fact) is { Ok: true } r) reply.Remembered.Add(r.Fact); // as stored, so "forget" finds it
            if (reply.Remembered.Count > 0 && section == "Chat") RenderNotes(reply);
        }
        catch (OperationCanceledException) { }
        catch (Exception ex) { Log(ex); }
        finally
        {
            if (memCts == cts) memCts = null;
            cts.Dispose();
            if (memWorking) { memWorking = false; memEndedAt = DateTime.Now; UpdateChatInput(); }
        }
    }


    /// While a command is being typed: the commands that match, like a shell's completion list.
    void RenderPalette()
    {
        if (chatPalette == null) return;
        chatPalette.Children.Clear();
        foreach (var (cmd, arg, what) in ChatLogic.Matches(chatDraft))
        {
            var t = new TextBlock { FontFamily = Mono, FontSize = 12 };
            t.Inlines.Add(new Run { Text = cmd, Foreground = B(Signal) });
            t.Inlines.Add(new Run { Text = arg.PadRight(20 - cmd.Length), Foreground = B(Dim) }); // descriptions line up
            t.Inlines.Add(new Run { Text = "  " + what, Foreground = B(Faint) });
            chatPalette.Children.Add(t);
        }
        if (chatPalette.Children.Count > 0) chatPalette.Children.Add(Txt("tab completes", 11, Faint, false, false));
        chatPalette.Visibility = chatPalette.Children.Count > 0 ? Visibility.Visible : Visibility.Collapsed;
    }

    /// "✦ remembered: …" under the answer it came from, with a way to take it back.
    void RenderNotes(ChatMsg m)
    {
        if (m.Notes == null) return;
        m.Notes.Children.Clear();
        foreach (var fact in m.Remembered.ToList())
        {
            var t = new TextBlock { FontFamily = Mono, FontSize = 11, TextWrapping = TextWrapping.Wrap, VerticalAlignment = VerticalAlignment.Center };
            t.Inlines.Add(new Run { Text = "✦ remembered  ", Foreground = B(Signal) });
            t.Inlines.Add(new Run { Text = fact, Foreground = B(Dim) });
            var forget = Btn("forget", Kind.Small, () => { Memory.Remove(fact); m.Remembered.Remove(fact); RenderNotes(m); UpdateChatInput(); });
            forget.BorderThickness = new Thickness(0); forget.Padding = new Thickness(6, 0, 6, 0);
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(forget, $"Forget: {fact}");
            m.Notes.Children.Add(Row(6, t, forget));
        }
        UpdateChatInput();
    }

    /// Memory: everything the chat remembers, editable, with the switches.
    async Task ShowMemory()
    {
        var list = Col(4);
        var usage = new BlockBar(8) { Width = 140, VerticalAlignment = VerticalAlignment.Center };
        var usageText = Txt("", 11, Faint, false, false);
        void Paint()
        {
            list.Children.Clear();
            var facts = Memory.Facts();
            if (facts.Count == 0) list.Children.Add(Txt("Nothing yet. Tell it about yourself in a chat, or add something below.", 12, Faint));
            foreach (var f in facts)
            {
                var x = Btn("×", Kind.Small, () => { Memory.Remove(f); Paint(); });
                x.BorderThickness = new Thickness(0); x.Padding = new Thickness(6, 0, 6, 0);
                Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(x, $"Forget: {f}");
                var g = new Grid { ColumnSpacing = 8 };
                g.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
                g.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
                g.Children.Add(Txt("• " + f, 12, Ink));
                Grid.SetColumn(x, 1); g.Children.Add(x);
                list.Children.Add(g);
            }
            var used = Memory.Used(facts);
            usage.SetFill(used / (double)Memory.Budget, used > Memory.Budget * 0.85 ? Gold : Signal);
            usageText.Text = $"{used} / {Memory.Budget} characters";
        }
        Paint();
        var box = new TextBox { PlaceholderText = "Add a fact, e.g. I'm vegetarian", FontFamily = Mono, FontSize = 12, CornerRadius = new CornerRadius(0), MinWidth = 330 };
        void AddIt() { var r = Memory.Add(box.Text); if (r.Ok) { box.Text = ""; Paint(); } else Toast(r.Why, Gold); }
        box.KeyDown += (_, e) => { if (e.Key == VirtualKey.Enter) { e.Handled = true; AddIt(); } };
        var how = new TextBox
        {
            Text = Memory.Instructions, PlaceholderText = "How should it answer? e.g. Keep answers short. Explain things like I'm new to them.",
            AcceptsReturn = true, TextWrapping = TextWrapping.Wrap, MaxLength = Memory.MaxInstructions, MinHeight = 64, MaxHeight = 140,
            FontFamily = Mono, FontSize = 12, CornerRadius = new CornerRadius(0),
        };
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(how, "How should it answer");
        how.LostFocus += (_, _) => Memory.Instructions = how.Text;
        var on = new CheckBox { Content = Txt("Use what it remembers in chats", 12), IsChecked = Memory.On };
        on.Click += (_, _) => Memory.On = on.IsChecked == true;
        var auto = new CheckBox { Content = Txt("Notice new things by itself (after a chat goes quiet)", 12), IsChecked = Memory.Auto };
        auto.Click += (_, _) => Memory.Auto = auto.IsChecked == true;
        var d = new ContentDialog
        {
            Title = "Memory",
            Content = new ScrollViewer
            {
                MaxHeight = 460,
                Content = Col(12,
                    Txt("What the chat remembers about you from earlier conversations. Your model reads it at the start of each new chat; it never leaves this computer.", 12, Dim),
                    list,
                    Row(8, box, Btn("Remember", Kind.Normal, AddIt)),
                    Row(8, usage, usageText),
                    on, auto,
                    Txt("Your instructions", 12, Ink, true).Also(t => t.Margin = new Thickness(0, 8, 0, 0)),
                    Txt("Sent at the start of every new chat. Leave empty for none.", 11, Faint),
                    how),
            },
            PrimaryButtonText = "Done", SecondaryButtonText = "Forget everything", DefaultButton = ContentDialogButton.Primary,
            XamlRoot = root.XamlRoot, RequestedTheme = ElementTheme.Dark,
        };
        memDialog = d;
        var r = await ShowDialog(d);
        memDialog = null;
        Memory.Instructions = how.Text; // also when Done is pressed while still typing
        if (r == ContentDialogResult.Secondary) { Memory.Clear(); Toast("Memory cleared.", Ok); }
        UpdateChatInput();
    }

    /// Developer check only: the demo can chat with a real test server given as
    /// --chat-server "url|key" (or "url|key|ctx" when it reads less than the
    /// demo model), to exercise the real streaming code.
    static readonly string[]? TestServer = TestArg();

    static string[]? TestArg()
    {
        var a = Environment.GetCommandLineArgs();
        int i = Array.IndexOf(a, "--chat-server");
        return Engine.Demo && i >= 0 && i + 1 < a.Length && a[i + 1].Split('|') is { Length: 2 or 3 } p ? p : null;
    }

    (ServerConfig? Config, string? Url) ChatTarget()
    {
        var c = status?.Config;
        if (TestServer == null || c == null) return (c, status?.LocalUrl);
        var ctx = TestServer.Length > 2 && int.TryParse(TestServer[2], out var n) ? n : c.Ctx;
        return (new ServerConfig { ModelId = c.ModelId, ModelName = c.ModelName, Ctx = ctx, Key = TestServer[1] }, TestServer[0]);
    }

    ContentDialog? memDialog;
    Windows.Media.Playback.MediaPlayer? speaker;
    ChatMsg? speaking;

    /// Reads an answer aloud with Windows' own voice; pressing again stops it.
    async Task Speak(ChatMsg m, Button b)
    {
        var label = (TextBlock)b.Content;
        if (speaking == m) { speaker?.Pause(); speaking = null; label.Text = "Listen"; return; }
        try
        {
            speaker ??= new Windows.Media.Playback.MediaPlayer();
            speaker.Pause();
            speaking = m;
            label.Text = "Stop";
            // Markdown marks aren't for reading out.
            var text = System.Text.RegularExpressions.Regex.Replace(m.Text.ToString(), @"```[\s\S]*?```", " (code) ");
            text = System.Text.RegularExpressions.Regex.Replace(text, @"[*_`#>|]+", " ");
            using var synth = new Windows.Media.SpeechSynthesis.SpeechSynthesizer();
            var stream = await synth.SynthesizeTextToStreamAsync(text.Length > 6000 ? text[..6000] : text);
            if (speaking != m) return;
            speaker.Source = Windows.Media.Core.MediaSource.CreateFromStream(stream, stream.ContentType);
            speaker.MediaEnded += Ended;
            speaker.Play();
            void Ended(Windows.Media.Playback.MediaPlayer p, object _)
            {
                p.MediaEnded -= Ended;
                Ui(() => { if (speaking == m) { speaking = null; label.Text = "Listen"; } });
            }
        }
        catch (Exception ex) { Log(ex); speaking = null; label.Text = "Listen"; Toast("This computer has no voice for reading aloud (add one in Windows Settings > Time & language > Speech).", Gold); }
    }

    /// The chat as plain text with markdown, to paste anywhere.
    string ChatText() => string.Join("\n\n", chat.Where(m => m.Text.Length > 0 || m.Files.Count > 0).Select(m =>
        (m.User ? "**You:** " : $"**{(m.Model != "" ? m.Model : ModelLabel)}:** ")
        + string.Concat(m.Files.Select(f => $"[{f.Name}] ")) + m.Text));

    /// The models already on this computer: pick one and the chat carries on with it.
    async Task ShowModels(FrameworkElement anchor)
    {
        await RefreshInstalled();
        var menu = new MenuFlyout();
        var current = status?.Config?.ModelId;
        foreach (var im in installed.GroupBy(i => i.ModelId).Select(g => g.First()))
        {
            var fits = im.Plan != null;
            var item = new MenuFlyoutItem { Text = (im.ModelId == current ? "● " : "   ") + im.Name + $"   {Fmt.Gb(im.Bytes)}" + (fits ? "" : "   (doesn't fit in free memory now)"), FontFamily = Mono, FontSize = 12, IsEnabled = fits && im.ModelId != current && !installRunning && chatCts == null };
            item.Click += (_, _) => _ = Use(im);
            menu.Items.Add(item);
        }
        if (installed.Count <= 1)
        {
            if (installed.Count == 1) menu.Items.Add(new MenuFlyoutSeparator());
            var more = new MenuFlyoutItem { Text = "Get another model in Models…", FontFamily = Mono, FontSize = 12 };
            more.Click += (_, _) => Go("Models");
            menu.Items.Add(more);
        }
        menu.ShowAt(anchor);
    }

    /// What the model gets for a question: its files, then the words.
    static string MessageContent(ChatMsg m) =>
        string.Concat(m.Files.Select(f => $"File: {f.Name}\n```\n{f.Text}\n```\n\n")) + m.Text;


    async Task PickFiles()
    {
        try
        {
            var picker = new Windows.Storage.Pickers.FileOpenPicker();
            WinRT.Interop.InitializeWithWindow.Initialize(picker, WinRT.Interop.WindowNative.GetWindowHandle(this));
            picker.FileTypeFilter.Add("*");
            var files = await picker.PickMultipleFilesAsync();
            if (files != null) Attach(files.Select(f => f.Path));
        }
        catch (Exception ex) { Log(ex); }
    }

    /// Reads text files to send with the next message; anything that isn't
    /// text, or wouldn't fit in the model's memory, is turned down with the reason.
    void Attach(IEnumerable<string> paths)
    {
        var max = status?.Config?.Ctx ?? 8192;
        foreach (var path in paths)
        {
            var name = Path.GetFileName(path);
            try
            {
                var info = new FileInfo(path);
                if (!info.Exists) continue; // a folder
                if (info.Length > 2_000_000) { Toast($"{name} is too big to attach ({info.Length / 1_000_000.0:0.#} MB). Copy in the part you need instead.", Gold); continue; }
                var bytes = File.ReadAllBytes(path);
                var decoded = DecodeText(bytes);
                if (decoded == null)
                {
                    Toast(name.EndsWith(".pdf", StringComparison.OrdinalIgnoreCase) || name.EndsWith(".docx", StringComparison.OrdinalIgnoreCase)
                        ? $"EZLM can't read {name} yet: PDF and Word files need their text copied in." : $"{name} isn't a text file, so the model can't read it.", Gold);
                    continue;
                }
                var text = decoded.TrimStart('\uFEFF');
                // Older turns make way when needed (Fit), but the files and question must fit on their own.
                int need = ChatLogic.Tokens(text) + chatFiles.Sum(f => ChatLogic.Tokens(f.Text)) + ChatLogic.Tokens(chatDraft);
                if (need > ChatLogic.Room(max) - 300) { Toast($"{name} is about {ChatLogic.Short(ChatLogic.Tokens(text))} tokens: more than {ModelLabel} can keep in mind ({ChatLogic.Short(max)}).", Gold); continue; }
                chatFiles.Add((name, text));
            }
            catch (Exception ex) { Log(ex); Toast($"Couldn't read {name}: {ex.Message}", Bad); }
        }
        RenderChips();
        chatBox?.Focus(FocusState.Programmatic);
    }

    /// Text in whatever form Windows saves it: UTF-8, UTF-16 (PowerShell output,
    /// registry exports) or the old Windows code page (Excel CSVs). Null for a
    /// file that isn't text.
    static string? DecodeText(byte[] b)
    {
        if (b.Length >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF) return Encoding.UTF8.GetString(b, 3, b.Length - 3);
        if (b.Length >= 2 && b[0] == 0xFF && b[1] == 0xFE) return Encoding.Unicode.GetString(b, 2, b.Length - 2);
        if (b.Length >= 2 && b[0] == 0xFE && b[1] == 0xFF) return Encoding.BigEndianUnicode.GetString(b, 2, b.Length - 2);
        int n = Math.Min(b.Length, 8000), zeros = 0, oddZeros = 0;
        for (int i = 0; i < n; i++) if (b[i] == 0) { zeros++; if (i % 2 == 1) oddZeros++; }
        if (zeros > 0)
            return n >= 4 && oddZeros * 10 >= n / 2 * 9 ? Encoding.Unicode.GetString(b) : null; // UTF-16 without a mark, or not text at all
        try { return new UTF8Encoding(false, true).GetString(b); }
        catch (DecoderFallbackException) { return Encoding.Latin1.GetString(b); } // close to Windows-1252: é, ü and £ survive
    }

    void RenderChips()
    {
        if (chatChips == null) return;
        chatChips.Children.Clear();
        foreach (var f in chatFiles.ToList()) chatChips.Children.Add(Chip(f.Name, f.Text, () => { chatFiles.Remove(f); RenderChips(); }));
        chatChips.Visibility = chatFiles.Count > 0 ? Visibility.Visible : Visibility.Collapsed;
    }

    /// A file as a small tag: name and size; with a remove button while it's still a draft.
    static UIElement Chip(string name, string text, Action? remove)
    {
        var row = Row(8, Txt("▤", 12, Signal, false, false), Txt(name, 12, Ink, false, false), Txt($"{ChatLogic.Short(ChatLogic.Tokens(text))} tokens", 11, Faint, false, false));
        if (remove != null)
        {
            var x = Btn("×", Kind.Small, remove);
            x.BorderThickness = new Thickness(0); x.Padding = new Thickness(4, 0, 2, 0);
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(x, $"Remove {name}");
            row.Children.Add(x);
        }
        foreach (var c in row.Children.OfType<FrameworkElement>()) c.VerticalAlignment = VerticalAlignment.Center;
        return new Border { Child = row, BorderBrush = B(Rule), BorderThickness = new Thickness(1), Padding = new Thickness(10, 4, 6, 4), Background = B(EZLM.Ui.Panel) };
    }

    /// Asks again for the last question: the old answer goes, a new one streams in.
    void RetryLast() => RetryLast(null);

    void RetryLast(bool? think)
    {
        // Only when it can actually ask again: never drop the question for nothing.
        var (c, url) = ChatTarget();
        if (chatCts != null || ServerState != 2 || c == null || string.IsNullOrEmpty(url) || chat.Count < 2 || chat[^1].User || !chat[^2].User) return;
        memCts?.Cancel();
        var q = chat[^2];
        chat.RemoveRange(chat.Count - 2, 2);
        if (section == "Chat") ShowSection();
        _ = SendChat(q.Text.ToString(), think, q.Files.ToList(), fromBox: false);
    }

    void SaveChat()
    {
        if (chat.Count == 0) return;
        // The first question, on one line, is the chat's name in History.
        var title = string.Join(" ", (chat.FirstOrDefault(m => m.User)?.Text.ToString() ?? "Chat").Split((char[]?)null, StringSplitOptions.RemoveEmptyEntries));
        if (title.Length > 60) title = title[..57] + "…";
        ChatStore.Save(new ChatStore.Saved
        {
            Id = chatId, Model = status?.Config?.ModelName ?? "", Updated = DateTime.Now, Title = title,
            Messages = chat.Select(m => new ChatStore.SavedMsg
            {
                User = m.User, Model = m.Model, Text = m.Text.ToString(), Thinking = m.Thinking.ToString(), Files = m.Files.Select(f => new ChatStore.SavedFile { Name = f.Name, Text = f.Text }).ToList(), Error = m.Error, Stopped = m.Stopped || (!m.User && !m.Done), Rate = m.Rate, Tokens = m.Tokens, Left = m.Left, Cut = m.Cut, // saved mid-answer: it stopped there
                Secs = m.Done && m.Ended > m.Started ? (m.Ended - m.Started).TotalSeconds : 0,
                ThinkSecs = m.ThinkEnd > m.ThinkStart ? (m.ThinkEnd - m.ThinkStart).TotalSeconds : 0,
            }).ToList(),
        });
    }

    void Restore(ChatStore.Saved s)
    {
        if (chatCts != null) SaveChat(); // keep the turn in progress
        chatCts?.Cancel();
        memCts?.Cancel();
        memFrom = 0;
        chatBriefing = null; chatInline = false; chatShare = 1;
        chat.Clear();
        chatId = s.Id;
        chatCtxUsed = 0;
        foreach (var x in s.Messages)
        {
            var m = new ChatMsg { User = x.User, Model = x.Model, Error = x.Error, Stopped = x.Stopped, Rate = x.Rate, Tokens = x.Tokens, Left = x.Left, Cut = x.Cut, Done = true, Files = x.Files.Select(f => (f.Name, f.Text)).ToList() };
            m.Text.Append(x.Text);
            m.Thinking.Append(x.Thinking);
            m.Started = DateTime.Now; m.Ended = m.Started + TimeSpan.FromSeconds(x.Secs);
            m.ThinkStart = DateTime.Now; m.ThinkEnd = m.ThinkStart + TimeSpan.FromSeconds(x.ThinkSecs);
            m.Shown = m.Text.Length;
            chat.Add(m);
        }
    }

    /// Past chats, newest first; pick one to carry on with it.
    void ShowHistory(FrameworkElement anchor)
    {
        var menu = new MenuFlyout();
        var saved = ChatStore.Recent();
        if (saved.Count == 0) menu.Items.Add(new MenuFlyoutItem { Text = "No saved chats yet", IsEnabled = false });
        foreach (var s in saved.Take(15))
        {
            var item = new MenuFlyoutItem { Text = $"{(s.Id == chatId ? "● " : "")}{s.Title}   ·   {ChatStore.Ago(s.Updated)}", FontFamily = Mono, FontSize = 12 };
            item.Click += (_, _) => { chatCts?.Cancel(); Restore(s); if (section == "Chat") ShowSection(); };
            menu.Items.Add(item);
        }
        if (saved.Count > 0)
        {
            menu.Items.Add(new MenuFlyoutSeparator());
            var copy = new MenuFlyoutItem { Text = "Copy this chat", FontFamily = Mono, FontSize = 12, IsEnabled = chat.Count > 0 };
            copy.Click += (_, _) => { Copy(ChatText()); Toast("Chat copied. Paste it anywhere.", Ok); };
            menu.Items.Add(copy);
            var del = new MenuFlyoutItem { Text = "Delete this chat", FontFamily = Mono, FontSize = 12, IsEnabled = chat.Count > 0 };
            del.Click += (_, _) => { ChatStore.Delete(chatId); NewChat(); };
            menu.Items.Add(del);
            var all = new MenuFlyoutItem { Text = "Delete all chats…", FontFamily = Mono, FontSize = 12 };
            all.Click += async (_, _) =>
            {
                if (await ShowDialog(Dialog("Delete all chats?", "Every saved chat is removed from this computer. This can't be undone.", "Delete all")) != ContentDialogResult.Primary) return;
                ChatStore.DeleteAll();
                NewChat();
            };
            menu.Items.Add(all);
        }
        menu.Items.Add(new MenuFlyoutSeparator());
        var web = new MenuFlyoutItem { Text = "Open llama.cpp's own chat in the browser", FontFamily = Mono, FontSize = 12 };
        web.Click += (_, _) => { if (status?.Config is { } c) _ = OpenChat(c); };
        menu.Items.Add(web);
        menu.ShowAt(anchor);
    }

}
