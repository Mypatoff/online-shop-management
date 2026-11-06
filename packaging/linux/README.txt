ShopKeeper for Linux
====================

Install (for your user, no root needed):

    ./install.sh

Then open "ShopKeeper" in your application menu. It starts in your web browser;
the "Quit" button at the top of the page stops it. Closing the tab or a terminal
does not.

Your data (shop.db, backups/) lives in ~/.local/share/shopkeeper/, next to the
program. Updating from inside the app, or running ./install.sh again from a newer
download, replaces only the program.

Remove:  ./uninstall.sh          (keeps your data)
         ./uninstall.sh --purge  (deletes it too)

Settings such as PORT, CURRENCY and SHOP_LANG are environment variables; see the
README of the project. To set them for the menu entry, edit
~/.local/share/applications/shopkeeper.desktop and put them in the Exec= line:

    Exec=env SHOP_LANG=uz "/home/you/.local/share/shopkeeper/shopkeeper"
